package api

import (
	"encoding/hex"

	"github.com/perlin-network/wavelet"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type LedgerStatus struct {
	PublicKey      string `json:"public_key"`
	Address        string `json:"address"`
	NumAccounts    uint64 `json:"num_accounts"`
	PreferredVotes int    `json:"preferred_votes"`
	SyncStatus     string `json:"sync_status"`

	Block     LedgerStatusBlock  `json:"block"`
	Preferred *LedgerStatusBlock `json:"preferred"`

	NumTx        int `json:"num_tx"`
	NumTxInStore int `json:"num_tx_in_store"`

	Peers []LedgerStatusPeer
}

type LedgerStatusBlock struct {
	MerkleRoot string `json:"merkle_root"` // [16]byte
	Height     uint64 `json:"height"`
	ID         string `json:"id"` // [32]byte
	Txs        int    `json:"transactions"`
}

type LedgerStatusPeer struct {
	Address   string `json:"address"`
	PublicKey string `json:"public_key"` // [32]byte
}

var _ JSONObject = (*LedgerStatus)(nil)

func (g *Gateway) ledgerStatus(ctx *fasthttp.RequestCtx) {
	var (
		snapshot  = g.Ledger.Snapshot()
		block     = g.Ledger.Blocks().Latest()
		publicKey = g.Keys.PublicKey()
	)

	var l = LedgerStatus{
		PublicKey:      hex.EncodeToString(publicKey[:]),
		Address:        g.Client.ID().Address(),
		NumAccounts:    wavelet.ReadAccountsLen(snapshot),
		PreferredVotes: g.Ledger.Finalizer().Progress(),
		SyncStatus:     g.Ledger.SyncStatus(),

		Block: LedgerStatusBlock{
			MerkleRoot: hex.EncodeToString(block.Merkle[:]),
			Height:     block.Index,
			ID:         hex.EncodeToString(block.ID[:]),
			Txs:        len(block.Transactions),
		},

		NumTx:        g.Ledger.Transactions().PendingLen(),
		NumTxInStore: g.Ledger.Transactions().Len(),
	}

	if preferred := g.Ledger.Finalizer().Preferred(); preferred != nil {
		b, ok := preferred.Value().(*wavelet.Block)
		if ok {
			l.Preferred = &LedgerStatusBlock{
				MerkleRoot: hex.EncodeToString(b.Merkle[:]),
				Height:     b.Index,
				ID:         hex.EncodeToString(b.ID[:]),
				Txs:        len(b.Transactions),
			}
		}
	}

	if peers := g.Client.ClosestPeerIDs(); len(peers) > 0 {
		l.Peers = make([]LedgerStatusPeer, len(peers))

		for i, p := range g.Client.ClosestPeerIDs() {
			pub := p.PublicKey()

			l.Peers[i].Address = p.Address()
			l.Peers[i].PublicKey = hex.EncodeToString(pub[:])
		}
	}

	g.render(ctx, &l)
}

func (s *LedgerStatus) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()

	arenaSet(arena, o, "public_key", s.PublicKey)
	arenaSet(arena, o, "address", s.Address)
	arenaSet(arena, o, "num_accounts", s.NumAccounts)
	arenaSet(arena, o, "preferred_votes", s.PreferredVotes)
	arenaSet(arena, o, "sync_status", s.SyncStatus)

	{
		block := arena.NewObject()

		arenaSet(arena, block, "merkle_root", s.Block.MerkleRoot)
		arenaSet(arena, block, "height", s.Block.Height)
		arenaSet(arena, block, "id", s.Block.ID)
		arenaSet(arena, block, "transactions", s.Block.Txs)

		o.Set("block", block)
	}

	if s.Preferred != nil {
		pref := arena.NewObject()

		arenaSet(arena, pref, "merkle_root", s.Preferred.MerkleRoot)
		arenaSet(arena, pref, "height", s.Preferred.Height)
		arenaSet(arena, pref, "id", s.Preferred.ID)
		arenaSet(arena, pref, "transactions", s.Preferred.Txs)

		o.Set("preferred", pref)
	}

	arenaSet(arena, o, "num_tx", s.NumTx)
	arenaSet(arena, o, "num_tx_in_store", s.NumTxInStore)

	var peers *fastjson.Value

	if len(s.Peers) > 0 {
		peers = arena.NewArray()

		for i, p := range s.Peers {
			peer := arena.NewObject()

			arenaSet(arena, peer, "address", p.Address)
			arenaSet(arena, peer, "public_key", p.PublicKey)

			peers.SetArrayItem(i, peer)
		}
	}

	o.Set("peers", peers)

	return o.MarshalTo(nil), nil
}

func (s *LedgerStatus) UnmarshalValue(v *fastjson.Value) error {
	s.PublicKey = valueString(v, "public_key")
	s.Address = valueString(v, "address")
	s.NumAccounts = v.GetUint64("num_accounts")
	s.PreferredVotes = v.GetInt("preferred_votes")
	s.SyncStatus = valueString(v, "sync_status")

	if preferred := v.GetObject("preferred"); preferred != nil {
		s.Preferred = &LedgerStatusBlock{
			MerkleRoot: "",
		}
	}
}
