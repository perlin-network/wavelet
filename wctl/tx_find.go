package wctl

import "errors"

// Find queries for either Account or Transaction. Either would be nil. If an
// error occurs, both will be nil.
func (c *Client) Find(address [32]byte) (*Account, *Transaction, error) {
	a, err := c.GetAccount(address)
	if err == nil {
		if a.Balance > 0 || a.Stake > 0 || a.Nonce > 0 || a.IsContract || a.NumPages > 0 {
			return a, nil, nil
		}
	}

	t, err := c.GetTransaction(address)
	if err == nil {
		return nil, t, nil
	}

	// return nil, nil, err
	return nil, nil, errors.New("address not found")
}
