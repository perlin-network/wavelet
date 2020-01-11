package wctl

import (
	"github.com/perlin-network/wavelet/api"
)

// GetLedgerStatus calls the /ledger endpoint of the API.
func (c *Client) LedgerStatus() (*api.LedgerStatus, error) {
	var res api.LedgerStatus

	if err := c.RequestJSON(RouteLedger, ReqGet, nil, &res); err != nil {
		return nil, err
	}

	return &res, nil
}
