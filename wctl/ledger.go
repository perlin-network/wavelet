package wctl

import (
	"net/url"
	"strconv"

	"github.com/valyala/fastjson"
)

var _ UnmarshalableJSON = (*LedgerStatusResponse)(nil)

// GetLedgerStatus calls the /ledger endpoint of the API.
func (c *Client) GetLedgerStatus(senderID string, creatorID string, offset uint64, limit uint64) (*LedgerStatusResponse, error) {
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

	var res LedgerStatusResponse
	if err := c.RequestJSON(path, ReqGet, nil, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

type LedgerStatusResponse struct {
	PublicKey     string   `json:"public_key"`
	HostAddress   string   `json:"address"`
	PeerAddresses []string `json:"peers"`

	RootID     string `json:"root_id"`
	RoundID    uint64 `json:"round_id"`
	Difficulty uint64 `json:"difficulty"`
}

func (l *LedgerStatusResponse) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	l.PublicKey = string(v.GetStringBytes("public_key"))
	l.HostAddress = string(v.GetStringBytes("address"))

	peerValue := v.GetArray("peers")
	for _, peer := range peerValue {
		l.PeerAddresses = append(l.PeerAddresses, peer.String())
	}

	l.RootID = string(v.GetStringBytes("root_id"))
	l.RoundID = v.GetUint64("round_id")
	l.Difficulty = v.GetUint64("difficulty")

	return nil
}
