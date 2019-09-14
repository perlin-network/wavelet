package wctl

import (
	"net/url"
	"strconv"

	"github.com/valyala/fastjson"
)

var _ UnmarshalableJSON = (*LedgerStatusResponse)(nil)

// GetLedgerStatus calls the /ledger endpoint of the API. All arguments are
// optional.
func (c *Client) LedgerStatus(senderID string, creatorID string, offset uint64, limit uint64) (*LedgerStatusResponse, error) {
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
	PublicKey      [32]byte `json:"public_key"`
	HostAddress    string   `json:"address"`
	NumAccounts    int      `json:"num_accounts"`
	PreferredVotes int      `json:"preferred_votes"`
	SyncStatus     string   `json:"sync_status"`
	PreferredID    string   `json:"preferred_id"` // optional

	Round struct {
		MerkleRoot [16]byte `json:"merkle_root"`
		StartID    [32]byte `json:"start_id"`
		EndID      [32]byte `json:"end_id"`
		Index      uint64   `json:"index"`
		Applied    uint64   `json:"applied"`
		Depth      uint64   `json:"depth"`
		Difficulty uint64   `json:"difficulty"`
	} `json:"round"`

	Graph struct {
		// TODO: make these int?
		Tx           uint64 `json:"num_tx"`
		MissingTx    uint64 `json:"num_missing_tx"`
		TxInStore    uint64 `json:"num_tx_in_store"`
		IncompleteTx uint64 `json:"num_incomplete_tx"`
		Height       uint64 `json:"height"`
	} `json:"graph"`

	Peers []Peer `json:"peers"`
}

type Peer struct {
	Address   string   `json:"address"`
	PublicKey [32]byte `json:"public_key"`
}

func (l *LedgerStatusResponse) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	if err := jsonHex(v, l.PublicKey[:], "public_key"); err != nil {
		return err
	}

	l.HostAddress = string(v.GetStringBytes("address"))
	l.NumAccounts = v.GetInt("num_accounts")
	l.PreferredVotes = v.GetInt("preferred_votes")
	l.SyncStatus = string(v.GetStringBytes("sync_status"))
	l.PreferredID = string(v.GetStringBytes("preferred_id"))

	if err := jsonHex(v, l.Round.MerkleRoot[:], "round", "merkle_root"); err != nil {
		return err
	}

	if err := jsonHex(v, l.Round.StartID[:], "round", "start_id"); err != nil {
		return err
	}

	if err := jsonHex(v, l.Round.EndID[:], "round", "end_id"); err != nil {
		return err
	}

	l.Round.Applied = v.GetUint64("round", "applied")
	l.Round.Depth = v.GetUint64("round", "depth")
	l.Round.Difficulty = v.GetUint64("round", "difficulty")

	l.Graph.Tx = v.GetUint64("graph", "num_tx")
	l.Graph.MissingTx = v.GetUint64("graph", "num_missing_tx")
	l.Graph.TxInStore = v.GetUint64("graph", "num_tx_in_store")
	l.Graph.IncompleteTx = v.GetUint64("graph", "num_incomplete_tx")
	l.Graph.Height = v.GetUint64("graph", "height")

	peerValue := v.GetArray("peers")
	l.Peers = make([]Peer, len(peerValue))

	for i, peer := range peerValue {
		l.Peers[i].Address = string(peer.GetStringBytes("address"))
		if err := jsonHex(peer, l.Peers[i].PublicKey[:], "public_key"); err != nil {
			return err
		}
	}

	return nil
}
