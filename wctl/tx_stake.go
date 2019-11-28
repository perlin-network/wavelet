package wctl

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/sys"
)

func (c *Client) PlaceStake(amount uint64) (*api.TxResponse, error) {
	return c.sendTransfer(byte(sys.TagStake), wavelet.Stake{
		Opcode: sys.PlaceStake,
		Amount: amount,
	})
}

func (c *Client) WithdrawStake(amount uint64) (*api.TxResponse, error) {
	return c.sendTransfer(byte(sys.TagStake), wavelet.Stake{
		Opcode: sys.WithdrawStake,
		Amount: amount,
	})
}

func (c *Client) WithdrawReward(amount uint64) (*api.TxResponse, error) {
	return c.sendTransfer(byte(sys.TagStake), wavelet.Stake{
		Opcode: sys.WithdrawReward,
		Amount: amount,
	})
}
