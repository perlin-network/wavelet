package wctl

import (
	"github.com/valyala/fastjson"
)

var _ UnmarshalableJSON = (*LedgerStatusResponse)(nil)

// GetLedgerStatus calls the /ledger endpoint of the API. All arguments are
// optional.
func (c *Client) LedgerStatus() (*LedgerStatusResponse, error) {
	var res LedgerStatusResponse

	if err := c.RequestJSON(RouteLedger, ReqGet, nil, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

type LedgerStatusResponse struct {
	PublicKey   [32]byte `json:"public_key"`
	HostAddress string   `json:"address"`
	NumAccounts int      `json:"num_accounts"`
	SyncStatus  string   `json:"sync_status"`

	Block struct {
		MerkleRoot [16]byte `json:"merkle_root"`
		Index      uint64   `json:"height"`
		ID         [32]byte `json:"id"`
		Txs        uint64   `json:"transactions"`
	} `json:"block"`

	NumTx        uint64 `json:"num_tx"`
	NumMissingTx uint64 `json:"num_missing_tx"`
	NumTxInStore uint64 `json:"num_tx_in_store"`
	AccountsLen  uint64 `json:"num_accounts_in_store"`

	Preferred *struct {
		MerkleRoot [16]byte `json:"merkle_root"`
		Index      uint64   `json:"height"`
		ID         [32]byte `json:"id"`
		Txs        uint64   `json:"transactions"`
	} `json:"preferred"`

	PreferredVotes int `json:"preferred_votes"`

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
	l.SyncStatus = string(v.GetStringBytes("sync_status"))

	{
		if err := jsonHex(v, l.Block.MerkleRoot[:], "block", "merkle_root"); err != nil {
			return err
		}

		l.Block.Index = v.GetUint64("block", "height")

		if err := jsonHex(v, l.Block.ID[:], "block", "id"); err != nil {
			return err
		}

		l.Block.Txs = v.GetUint64("block", "transactions")
	}

	l.NumTx = v.GetUint64("num_tx")
	l.NumMissingTx = v.GetUint64("num_missing_tx")
	l.NumTxInStore = v.GetUint64("num_tx_in_store")

	if v.Exists("preferred") && v.Get("preferred").Type() != fastjson.TypeNull {
		l.Preferred = &struct {
			MerkleRoot [16]byte `json:"merkle_root"`
			Index      uint64   `json:"height"`
			ID         [32]byte `json:"id"`
			Txs        uint64   `json:"transactions"`
		}{}

		if err := jsonHex(v, l.Preferred.MerkleRoot[:], "preferred", "merkle_root"); err != nil {
			return err
		}

		l.Preferred.Index = v.GetUint64("preferred", "height")

		if err := jsonHex(v, l.Preferred.ID[:], "preferred", "id"); err != nil {
			return err
		}

		l.Preferred.Txs = v.GetUint64("preferred", "transactions")
	}

	l.PreferredVotes = v.GetInt("preferred_votes")

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
