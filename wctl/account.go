package wctl

import (
	"encoding/hex"

	"github.com/valyala/fastjson"
)

var _ UnmarshalableJSON = (*Account)(nil)

// GetSelf gets the current account.
func (c *Client) GetSelf() (*Account, error) {
	return c.GetAccount(c.PublicKey)
}

// GetAccount calls the /accounts endpoint of the API.
func (c *Client) GetAccount(account [32]byte) (*Account, error) {
	path := RouteAccount + "/" + hex.EncodeToString(account[:])

	var res Account
	if err := c.RequestJSON(path, ReqGet, nil, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

// Convenient function for a.IsContract
func (c *Client) RecipientIsContract(recipient [32]byte) bool {
	a, err := c.GetAccount(recipient)
	if err != nil {
		return false
	}

	return a.IsContract
}

type Account struct {
	PublicKey  [32]byte `json:"public_key"`
	Balance    uint64   `json:"balance"`
	GasBalance uint64   `json:"gas_balance"`
	Stake      uint64   `json:"stake"`
	Reward     uint64   `json:"reward"`
	Nonce      uint64   `json:"nonce"`
	IsContract bool     `json:"is_contract"`
	NumPages   uint64   `json:"num_mem_pages,omitempty"`
}

func (a *Account) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	if err := jsonHex(v, a.PublicKey[:], "public_key"); err != nil {
		return err
	}

	a.Balance = v.GetUint64("balance")
	a.GasBalance = v.GetUint64("gas_balance")
	a.Stake = v.GetUint64("stake")
	a.Reward = v.GetUint64("reward")
	a.Nonce = v.GetUint64("nonce")
	a.IsContract = v.GetBool("is_contract")
	a.NumPages = v.GetUint64("num_mem_pages")

	return nil
}
