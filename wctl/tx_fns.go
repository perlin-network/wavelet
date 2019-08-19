package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
)

func (c *Client) Pay(recipient [32]byte, amount uint64) (*TxResponse, error) {
	a, err := c.GetAccount(recipient)
	if err != nil {
		return nil, err
	}

	var t wavelet.Transfer

	// Write the recipient
	copy(t.Recipient[:], recipient[:])

	// Set the amount
	t.Amount = amount

	if a.Balance < amount+sys.TransactionFeeAmount {
		return nil, ErrInsufficientPerls
	}

	if a.IsContract {
		// Set the contract parameters
		t.GasLimit = a.Balance - amount - sys.TransactionFeeAmount
		t.FuncName = []byte("on_money_received")
	}

	return c.sendTransfer(byte(sys.TagTransfer), t)
}

// TODO(diamond): Port the old function call API over
func (c *Client) Call(recipient [32]byte)
