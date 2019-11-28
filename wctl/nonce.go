package wctl

import (
	"encoding/hex"

	"github.com/perlin-network/wavelet/api"
)

func (c *Client) GetAccountNonce(account [32]byte) (*api.NonceResponse, error) {
	path := RouteNonce + "/" + hex.EncodeToString(account[:])

	var res api.NonceResponse
	if err := c.RequestJSON(path, ReqGet, nil, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

func (c *Client) GetSelfNonce() (*api.NonceResponse, error) {
	return c.GetAccountNonce(c.PublicKey)
}
