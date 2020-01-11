package wctl

import (
<<<<<<< HEAD
	"net/url"
	"strconv"

	"github.com/perlin-network/wavelet/api"
=======
	"github.com/valyala/fastjson"
>>>>>>> f59047e31aa71fc9fbecf1364e37a5e5641f8e01
)

// GetLedgerStatus calls the /ledger endpoint of the API. All arguments are
// optional.
<<<<<<< HEAD
func (c *Client) LedgerStatus(senderID string, creatorID string, offset uint64,
	limit uint64) (*api.LedgerStatus, error) {

	vals := url.Values{}

	if senderID != "" {
		vals.Set("sender", senderID)
	}

	if creatorID != "" {
		vals.Set("creator", creatorID)
	}

	if offset != 0 {
		vals.Set("offset", strconv.FormatUint(offset, 10))
	}

	if limit != 0 {
		vals.Set("limit", strconv.FormatUint(limit, 10))
	}

	path := RouteLedger + "?" + vals.Encode()

	var res api.LedgerStatus

	if err := c.RequestJSON(path, ReqGet, nil, &res); err != nil {
=======
func (c *Client) LedgerStatus() (*LedgerStatusResponse, error) {
	var res LedgerStatusResponse

	if err := c.RequestJSON(RouteLedger, ReqGet, nil, &res); err != nil {
>>>>>>> f59047e31aa71fc9fbecf1364e37a5e5641f8e01
		return nil, err
	}

	return &res, nil
}
