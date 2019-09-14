package wctl

// Find queries for either Account or Transaction. Either would be nil. If an
// error occurs, both will be nil.
func (c *Client) Find(address [32]byte) (*Account, *Transaction, error) {
	a, err := c.GetAccount(address)
	if err == nil {
		return a, nil, nil
	}

	t, err := c.GetTransaction(address)
	if err == nil {
		return nil, t, nil
	}

	return nil, nil, err
}
