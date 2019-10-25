package api

import (
	"encoding/hex"
	"strconv"

	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type nonceResponse struct {
	Nonce uint64 `json:"nonce"`
	Block uint64 `json:"block"`
}

func (g *Gateway) getAccountNonce(ctx *fasthttp.RequestCtx) {
	param, ok := ctx.UserValue("id").(string)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("id must be a string")))
		return
	}

	slice, err := hex.DecodeString(param)
	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "account ID must be presented as valid hex")))
		return
	}

	if len(slice) != wavelet.SizeAccountID {
		g.renderError(ctx, ErrBadRequest(errors.Errorf("account ID must be %d bytes long", wavelet.SizeAccountID)))
		return
	}

	var id wavelet.AccountID
	copy(id[:], slice)

	snapshot := g.Ledger.Snapshot()
	nonce, _ := wavelet.ReadAccountNonce(snapshot, id)
	block := g.Ledger.Blocks().Latest().Index

	g.render(ctx, &nonceResponse{Nonce: nonce, Block: block})
}

func (s *nonceResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	o.Set("nonce", arena.NewNumberString(strconv.FormatUint(s.Nonce, 10)))
	o.Set("block", arena.NewNumberString(strconv.FormatUint(s.Block, 10)))
	return o.MarshalTo(nil), nil
}
