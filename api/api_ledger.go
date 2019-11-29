package api

import (
	"encoding/hex"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type LedgerStatus struct {
	PublicKey      edwards25519.PublicKey `json:"public_key"`
	Address        string                 `json:"address"`
	NumAccounts    uint64                 `json:"num_accounts"`
	PreferredVotes int                    `json:"preferred_votes"`
	SyncStatus     string                 `json:"sync_status"`

	Block     LedgerStatusBlock  `json:"block"`
	Preferred *LedgerStatusBlock `json:"preferred"`

	NumTx        int `json:"num_tx"`
	NumTxInStore int `json:"num_tx_in_store"`

	Peers []LedgerStatusPeer
}

type LedgerStatusBlock struct {
	MerkleRoot wavelet.MerkleNodeID `json:"merkle_root"`
	Height     uint64               `json:"height"`
	ID         wavelet.BlockID      `json:"id"`
	Txs        int                  `json:"transactions"`
}

type LedgerStatusPeer struct {
	Address   string                 `json:"address"`
	PublicKey edwards25519.PublicKey `json:"public_key"`
}

var _ log.JSONObject = (*LedgerStatus)(nil)

func (g *Gateway) ledgerStatus(ctx *fasthttp.RequestCtx) {
	var (
		snapshot  = g.Ledger.Snapshot()
		block     = g.Ledger.Blocks().Latest()
		publicKey = g.Keys.PublicKey()
	)

	var l = LedgerStatus{
		PublicKey:      publicKey,
		Address:        g.Client.ID().Address(),
		NumAccounts:    wavelet.ReadAccountsLen(snapshot),
		PreferredVotes: g.Ledger.Finalizer().Progress(),
		SyncStatus:     g.Ledger.SyncStatus(),

		Block: LedgerStatusBlock{
			MerkleRoot: block.Merkle,
			Height:     block.Index,
			ID:         block.ID,
			Txs:        len(block.Transactions),
		},

		NumTx:        g.Ledger.Transactions().PendingLen(),
		NumTxInStore: g.Ledger.Transactions().Len(),
	}

	if preferred := g.Ledger.Finalizer().Preferred(); preferred != nil {
		b, ok := preferred.Value().(*wavelet.Block)
		if ok {
			l.Preferred = &LedgerStatusBlock{
				MerkleRoot: b.Merkle,
				Height:     b.Index,
				ID:         b.ID,
				Txs:        len(b.Transactions),
			}
		}
	}

	if peers := g.Client.ClosestPeerIDs(); len(peers) > 0 {
		l.Peers = make([]LedgerStatusPeer, len(peers))

		for i, p := range g.Client.ClosestPeerIDs() {
			pub := p.PublicKey()

			l.Peers[i].Address = p.Address()
			l.Peers[i].PublicKey = pub
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
	if err := log.ValueHex(v, s.PublicKey[:], "public_key"); err != nil {
		return err
	}

	s.Address = log.ValueString(v, "address")
	s.NumAccounts = v.GetUint64("num_accounts")
	s.PreferredVotes = v.GetInt("preferred_votes")
	s.SyncStatus = log.ValueString(v, "sync_status")

	// Parse block

	log.ValueHex(v, s.Block.MerkleRoot, "block", "merkle_root")
	log.ValueHex(v, s.Block.ID, "block", "id")
	s.Block.Height = v.GetUint64("block", "height")
	s.Block.Txs = v.GetInt("block", "transactions")

	// Parse preferred block

	if v.Exists("preferred") {
		s.Preferred = &LedgerStatusBlock{}

		log.ValueHex(v, s.Preferred.MerkleRoot, "preferred", "merkle_root")
		log.ValueHex(v, s.Preferred.ID, "preferred", "id")
		s.Preferred.Height = v.GetUint64("preferred", "height")
		s.Preferred.Txs = v.GetInt("preferred", "transactions")
	}

	s.NumTx = v.GetInt("num_tx")
	s.NumTxInStore = v.GetInt("num_tx_in_store")

	var peers = v.GetArray("peers")

	if len(peers) > 0 {
		s.Peers = make([]LedgerStatusPeer, len(peers))

		for i, v := range peers {
			s.Peers[i].Address = log.ValueString(v, "address")
			log.ValueHex(v, s.Peers[i].PublicKey, "public_key")
		}
	}

	return nil
}

func (s *LedgerStatus) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("public_key", s.PublicKey[:])
	ev.Str("address", s.Address)
	ev.Uint64("num_accounts", s.NumAccounts)
	ev.Int("preferred_votes", s.PreferredVotes)
	ev.Str("sync_status", s.SyncStatus)

	ev.Hex("block_merkle_root", s.Block.MerkleRoot[:])
	ev.Hex("block_id", s.Block.ID[:])
	ev.Uint64("block_height", s.Block.Height)
	ev.Int("block_transactions", s.Block.Txs)

	if s.Preferred != nil {
		ev.Hex("preferred_merkle_root", s.Preferred.MerkleRoot[:])
		ev.Hex("preferred_id", s.Preferred.ID[:])
		ev.Uint64("preferred_height", s.Preferred.Height)
		ev.Int("preferred_transactions", s.Preferred.Txs)
	}

	ev.Int("num_tx", s.NumTx)
	ev.Int("num_tx_in_store", s.NumTxInStore)

	var peers = make([]string, len(s.Peers))
	for i, p := range s.Peers {
		peers[i] = p.Address +
			"[" + hex.EncodeToString(p.PublicKey[:]) + "]"
	}
	ev.Strs("peers", peers)

	ev.Msg("Here is the current status of your node.")
}
