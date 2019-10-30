package api

import (
	"encoding/base64"
	"encoding/hex"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type TxResponse struct {
	ID wavelet.TransactionID `json:"id"`
}

var _ log.JSONObject = (*TxResponse)(nil)

func (g *Gateway) sendTransaction(ctx *fasthttp.RequestCtx) {
	req := new(TxRequest)

	if g.Ledger != nil && g.Ledger.TakeSendQuota() == false {
		g.renderError(ctx, ErrInternal(errors.New("rate limit")))
		return
	}

	parser := g.parserPool.Get()
	defer g.parserPool.Put(parser)

	if err := req.bind(parser, ctx.PostBody()); err != nil {
		g.renderError(ctx, ErrBadRequest(err))
		return
	}

	tx := wavelet.NewSignedTransaction(
		req.Sender, req.Nonce, req.Block,
		sys.Tag(req.Tag), req.Payload, req.Signature,
	)

	// TODO(kenta): check signature and nonce

	if err := g.Ledger.AddTransaction(tx); err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(err, "error adding your transaction to graph")))
		return
	}

	g.render(ctx, &TxResponse{
		ID: tx.ID,
	})
}

func (s *TxResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	arenaSet(arena, o, "id", s.ID)

	return o.MarshalTo(nil), nil
}

func (s *TxResponse) UnmarshalValue(v *fastjson.Value) error {
	return valueHex(v, s.ID, "id")
}

func (s *TxResponse) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("id", s.ID[:])
	ev.Msg("Transaction sent.")
}

/*
	Get Transaction
*/

type Transaction struct {
	ID     wavelet.TransactionID `json:"id"`
	Sender wavelet.AccountID     `json:"sender"`
	// Status    string `json:"status"`
	Nonce     uint64            `json:"nonce"`
	Tag       uint8             `json:"tag"`
	Payload   []byte            `json:"payload"` // base64
	Signature wavelet.Signature `json:"signature"`
}

var _ log.JSONObject = (*Transaction)(nil)

func (g *Gateway) getTransaction(ctx *fasthttp.RequestCtx) {
	param, ok := ctx.UserValue("id").(string)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("id must be a string")))
		return
	}

	slice, err := hex.DecodeString(param)
	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "transaction ID must be presented as valid hex")))
		return
	}

	if len(slice) != wavelet.SizeTransactionID {
		g.renderError(ctx, ErrBadRequest(errors.Errorf("transaction ID must be %d bytes long", wavelet.SizeTransactionID)))
		return
	}

	var id wavelet.TransactionID
	copy(id[:], slice)

	tx := g.Ledger.Transactions().Find(id)
	if tx == nil {
		g.renderError(ctx, ErrNotFound(errors.Errorf("could not find transaction with ID %x", id)))
		return
	}

	g.render(ctx, &Transaction{
		ID:        tx.ID,
		Sender:    tx.Sender,
		Nonce:     tx.Nonce,
		Tag:       uint8(tx.Tag),
		Payload:   tx.Payload,
		Signature: tx.Signature,
	})
}

func (s *Transaction) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o, err := s.getObject(arena)
	if err != nil {
		return nil, err
	}

	return o.MarshalTo(nil), nil
}

func (s *Transaction) getObject(arena *fastjson.Arena) (*fastjson.Value, error) {
	o := arena.NewObject()

	arenaSets(arena, o,
		"id", s.ID,
		"sender", s.Sender,
		"nonce", s.Nonce,
		"tag", s.Tag,
		"payload", base64.StdEncoding.EncodeToString(s.Payload),
		"signature", s.Signature,
	)

	return o, nil
}

func (s *Transaction) UnmarshalValue(v *fastjson.Value) error {
	valueHex(v, s.ID, "id")
	valueHex(v, s.Sender, "sender")
	valueHex(v, s.Signature, "signature")

	s.Nonce = v.GetUint64("nonce")
	s.Tag = uint8(v.GetUint("tag"))

	pl, _ := valueBase64(v, "payload")
	s.Payload = pl

	return nil
}

// MarshalEvent doesn't return the full contents of Payload, but only its
// length.
func (s *Transaction) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("id", s.ID[:])
	ev.Hex("sender", s.Sender[:])
	ev.Hex("signature", s.Signature[:])

	ev.Uint64("nonce", s.Nonce)
	ev.Uint8("tag", s.Tag)

	ev.Int("payload_len", len(s.Payload))

	ev.Msg("Transaction")
}

type TransactionList []*Transaction

// var _ log.JSONObject = (TransactionList)(nil)

func (g *Gateway) listTransactions(ctx *fasthttp.RequestCtx) {
	// TODO

	/*
		var sender wavelet.AccountID
		var offset, limit uint64
		var err error

		queryArgs := ctx.QueryArgs()
		if raw := string(queryArgs.Peek("sender")); len(raw) > 0 {
			slice, err := hex.DecodeString(raw)
			if err != nil {
				g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "sender ID must be presented as valid hex")))
				return
			}

			if len(slice) != wavelet.SizeAccountID {
				g.renderError(ctx, ErrBadRequest(errors.Errorf("sender ID must be %d bytes long", wavelet.SizeAccountID)))
				return
			}

			copy(sender[:], slice)
		}

		if raw := string(queryArgs.Peek("offset")); len(raw) > 0 {
			offset, err = strconv.ParseUint(raw, 10, 64)

			if err != nil {
				g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "could not parse offset")))
				return
			}
		}

		if raw := string(queryArgs.Peek("limit")); len(raw) > 0 {
			limit, err = strconv.ParseUint(raw, 10, 64)

			if err != nil {
				g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "could not parse limit")))
				return
			}
		}

		if limit > maxPaginationLimit {
			limit = maxPaginationLimit
		}

	*/

	var transactions TransactionList

	/*

		g.Ledger.Transactions().Iterate
		var rootDepth = g.Ledger.Graph().RootDepth()

		for _, tx := range g.Ledger.Graph().ListTransactions(offset, limit, sender) {
			status := "received"

			if tx.Depth <= rootDepth {
				status = "applied"
			}

			transactions = append(transactions, &transaction{tx: tx, status: status})
		}

	*/

	g.render(ctx, transactions)
}

func (s TransactionList) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	list := arena.NewArray()

	for i, v := range s {
		o, err := v.getObject(arena)
		if err != nil {
			return nil, err
		}

		list.SetArrayItem(i, o)
	}

	return list.MarshalTo(nil), nil
}
