package api

import (
	"encoding/hex"
	"strconv"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type ledgerStatusResponse struct {
	// Internal fields.

	client    *skademlia.Client
	ledger    *wavelet.Ledger
	publicKey edwards25519.PublicKey
}

var _ marshalableJSON = (*ledgerStatusResponse)(nil)

func (g *Gateway) ledgerStatus(ctx *fasthttp.RequestCtx) {
	g.render(ctx, &ledgerStatusResponse{
		client:    g.Client,
		ledger:    g.Ledger,
		publicKey: g.Keys.PublicKey(),
	})
}

func (s *ledgerStatusResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	if s.client == nil || s.ledger == nil {
		return nil, errors.New("insufficient parameters were provided")
	}

	snapshot := s.ledger.Snapshot()
	block := s.ledger.Blocks().Latest()
	preferred := s.ledger.Finalizer().Preferred()

	accountsLen := wavelet.ReadAccountsLen(snapshot)

	o := arena.NewObject()

	o.Set("public_key",
		arena.NewString(hex.EncodeToString(s.publicKey[:])))
	o.Set("address",
		arena.NewString(s.client.ID().Address()))
	o.Set("num_accounts",
		arena.NewNumberString(strconv.FormatUint(accountsLen, 10)))
	o.Set("preferred_votes",
		arena.NewNumberInt(s.ledger.Finalizer().Progress()))
	o.Set("sync_status",
		arena.NewString(s.ledger.SyncStatus()))

	{
		blockObj := arena.NewObject()
		blockObj.Set("merkle_root",
			arena.NewString(hex.EncodeToString(block.Merkle[:])))
		blockObj.Set("height",
			arena.NewNumberString(strconv.FormatUint(block.Index, 10)))
		blockObj.Set("id",
			arena.NewString(hex.EncodeToString(block.ID[:])))
		blockObj.Set("transactions",
			arena.NewNumberInt(len(block.Transactions)))

		o.Set("block", blockObj)
	}

	if preferred != nil {
		preferredObj := arena.NewObject()
		preferredObj.Set("merkle_root",
			arena.NewString(hex.EncodeToString(block.Merkle[:])))
		preferredObj.Set("height",
			arena.NewNumberString(strconv.FormatUint(block.Index, 10)))
		preferredObj.Set("id",
			arena.NewString(hex.EncodeToString(block.ID[:])))
		preferredObj.Set("transactions",
			arena.NewNumberInt(len(block.Transactions)))

		o.Set("preferred", preferredObj)
	} else {
		o.Set("preferred", arena.NewNull())
	}

	o.Set("num_tx",
		arena.NewNumberInt(s.ledger.Transactions().PendingLen()))
	o.Set("num_tx_in_store",
		arena.NewNumberInt(s.ledger.Transactions().Len()))
	o.Set("num_accounts_in_store",
		arena.NewNumberString(strconv.FormatUint(accountsLen, 10)))

	peers := s.client.ClosestPeerIDs()
	if len(peers) > 0 {
		peersArray := arena.NewArray()

		for i := range peers {
			publicKey := peers[i].PublicKey()

			peer := arena.NewObject()
			peer.Set("address", arena.NewString(peers[i].Address()))
			peer.Set("public_key", arena.NewString(hex.EncodeToString(publicKey[:])))

			peersArray.SetArrayItem(i, peer)
		}
		o.Set("peers", peersArray)
	} else {
		o.Set("peers", nil)
	}

	return o.MarshalTo(nil), nil
}
