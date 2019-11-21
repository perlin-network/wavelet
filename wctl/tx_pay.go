package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
)

func (c *Client) Pay(recipient [32]byte, amount uint64) (*TxResponse, error) {
	a, err := c.GetSelf()
	if err != nil {
		return nil, err
	}

	var t wavelet.Transfer

	// Write the recipient
	t.Recipient = recipient

	// Set the amount
	t.Amount = amount

	if a.Balance < amount+4*1024*1024 {
		return nil, ErrInsufficientPerls
	}

	recipientAccount, err := c.GetAccount(recipient)
	if err != nil {
		return nil, err
	}

	if recipientAccount.IsContract {
		// Set the contract parameters
		t.GasLimit = a.Balance - amount - 4*1024*1024
		t.FuncName = []byte("on_money_received")
	}

	return c.sendTransfer(byte(sys.TagTransfer), t)
}
