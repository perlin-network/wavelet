package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
)

func (c *Client) DepositGas(recipient [32]byte, gasAmount uint64) (*TxResponse, error) {
	a, err := c.GetAccount(recipient)
	if err != nil {
		return nil, err
	}

	if !a.IsContract {
		return nil, ErrNotContract
	}

	if a.Balance < gasAmount+sys.TransactionFeeAmount {
		return nil, ErrInsufficientPerls
	}

	return c.sendTransfer(byte(sys.TagTransfer), wavelet.Transfer{
		Recipient:  recipient,
		GasDeposit: gasAmount,
	})
}
