package wctl

import (
	"errors"

	"github.com/perlin-network/wavelet/api"
)

// Find queries for either Account or Transaction. Either would be nil. If an
// error occurs, both will be nil.
<<<<<<< HEAD
func (c *Client) Find(address [32]byte) (*api.Account, *api.Transaction, error) {
=======
func (c *Client) Find(address [32]byte) (*Account, *Transaction, error) {
	a, err := c.GetAccount(address)
	if err == nil {
		if a.Balance > 0 || a.Stake > 0 || a.IsContract || a.NumPages > 0 {
			return a, nil, nil
		}
	}

>>>>>>> f59047e31aa71fc9fbecf1364e37a5e5641f8e01
	t, err := c.GetTransaction(address)
	if err == nil {
		return nil, t, nil
	}

	a, err := c.GetAccount(address)
	if err != nil {
		return nil, nil, err
	}

	if a.Balance > 0 || a.Stake > 0 || a.Nonce > 0 || a.IsContract || a.NumPages > 0 {
		return a, nil, nil
	}

	// return nil, nil, err
	return nil, nil, errors.New("Address not found")
}
