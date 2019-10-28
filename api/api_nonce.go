package api

import (
	"encoding/hex"

	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type NonceResponse struct {
	Nonce uint64 `json:"nonce"`
	Block uint64 `json:"block"`
}

var _ JSONObject = (*NonceResponse)(nil)

func (g *Gateway) getAccountNonce(ctx *fasthttp.RequestCtx) {
	param, ok := ctx.UserValue("id").(string)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("id must be a string")))
		return
	}

	slice, err := hex.DecodeString(param)
	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(
			err, "account ID must be presented as valid hex")))
		return
	}

	if len(slice) != wavelet.SizeAccountID {
		g.renderError(ctx, ErrBadRequest(errors.Errorf(
			"account ID must be %d bytes long", wavelet.SizeAccountID)))
		return
	}

	var id wavelet.AccountID
	copy(id[:], slice)

	snapshot := g.Ledger.Snapshot()
	nonce, _ := wavelet.ReadAccountNonce(snapshot, id)
	block := g.Ledger.Blocks().Latest().Index

	g.render(ctx, &NonceResponse{
		Nonce: nonce, Block: block,
	})
}

func (s *NonceResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	arenaSet(arena, o, "nonce", s.Nonce)
	arenaSet(arena, o, "block", s.Block)
	return o.MarshalTo(nil), nil
}

func (s *NonceResponse) UnmarshalValue(v *fastjson.Value) error {
	s.Nonce = v.GetUint64("nonce")
	s.Block = v.GetUint64("block")
	return nil
}
