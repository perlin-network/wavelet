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
