package wctl

import "log"

// Find queries for either Account or Transaction. Either would be nil. If an
// error occurs, both will be nil.
func (c *Client) Find(address [32]byte) (*Account, *Transaction, error) {
	t, err := c.GetTransaction(address)
	if err == nil {
		return nil, t, nil
	}

	log.Println(err)

	a, err := c.GetAccount(address)
	if err == nil {
		return a, nil, nil
	}

	return nil, nil, err
}
