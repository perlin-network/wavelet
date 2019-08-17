package wctl

import (
	"github.com/valyala/fastjson"
)

var _ UnmarshalableJSON = (*Account)(nil)

// GetAccount calls the /accounts endpoint of the API.
func (c *Client) GetAccount(accountID string) (*Account, error) {
	path := RouteAccount + "/" + accountID

	var res Account
	if err := c.RequestJSON(path, ReqGet, nil, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

type Account struct {
	PublicKey string `json:"public_key"`
	Balance   uint64 `json:"balance"`
	Stake     uint64 `json:"stake"`

	IsContract bool   `json:"is_contract"`
	NumPages   uint64 `json:"num_mem_pages,omitempty"`
}

func (a *Account) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	a.PublicKey = string(v.GetStringBytes("public_key"))
	a.Balance = v.GetUint64("balance")
	a.Stake = v.GetUint64("stake")
	a.IsContract = v.GetBool("is_contract")
	a.NumPages = v.GetUint64("num_mem_pages")

	return nil
}
