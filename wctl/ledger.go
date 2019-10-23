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

	Block struct {
		MerkleRoot [16]byte `json:"merkle_root"`
		Index      uint64   `json:"height"`
		ID         [32]byte `json:"id"`
		Txs        uint64   `json:"transactions"`
	} `json:"block"`

	NumTx        uint64 `json:"num_tx"`
	NumTxInStore uint64 `json:"num_tx_in_store"`
	AccountsLen  uint64 `json:"num_accounts_in_store"`

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

	if err := jsonHex(v, l.Block.MerkleRoot[:], "block", "merkle_root"); err != nil {
		return err
	}

	if err := jsonHex(v, l.Block.ID[:], "block", "id"); err != nil {
		return err
	}

	l.Block.Txs = v.GetUint64("block", "transactions")

	l.NumTx = v.GetUint64("num_tx")
	l.NumTxInStore = v.GetUint64("num_tx_in_store")

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
