package wctl

import (
	"encoding/hex"

	"github.com/perlin-network/wavelet/api"
)

// GetSelf gets the current account.
func (c *Client) GetSelf() (*api.Account, error) {
	return c.GetAccount(c.PublicKey)
}

// GetAccount calls the /accounts endpoint of the API.
func (c *Client) GetAccount(account [32]byte) (*api.Account, error) {
	path := RouteAccount + "/" + hex.EncodeToString(account[:])

	var res api.Account
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
<<<<<<< HEAD
=======

type Account struct {
	PublicKey  [32]byte `json:"public_key"`
	Balance    uint64   `json:"balance"`
	GasBalance uint64   `json:"gas_balance"`
	Stake      uint64   `json:"stake"`
	Reward     uint64   `json:"reward"`
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
	a.IsContract = v.GetBool("is_contract")
	a.NumPages = v.GetUint64("num_mem_pages")

	return nil
}
>>>>>>> f59047e31aa71fc9fbecf1364e37a5e5641f8e01
