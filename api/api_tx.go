package api

import (
	"encoding/base64"
	"encoding/hex"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type TxResponse struct {
	ID string
}

var _ JSONObject = (*TxResponse)(nil)

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
		req.sender, req.Nonce, req.Block,
		sys.Tag(req.Tag), req.payload, req.signature,
	)

	// TODO(kenta): check signature and nonce

	if err := g.Ledger.AddTransaction(tx); err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(err, "error adding your transaction to graph")))
		return
	}

	g.render(ctx, &TxResponse{
		ID: hex.EncodeToString(tx.ID[:]),
	})
}

func (s *TxResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	arenaSet(arena, o, "id", s.ID)

	return o.MarshalTo(nil), nil
}

/*
	Get Transaction
*/

type Transaction struct {
	ID     string `json:"id"`     // [32]byte
	Sender string `json:"sender"` // [32]byte
	// Status    string `json:"status"`
	Nonce     uint64 `json:"nonce"`
	Tag       uint8  `json:"tag"`
	Payload   string `json:"payload"`   // []byte base64
	Signature string `json:"signature"` // [64]byte
}

var _ JSONObject = (*Transaction)(nil)

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
		ID:        hex.EncodeToString(tx.ID[:]),
		Sender:    hex.EncodeToString(tx.Sender[:]),
		Nonce:     tx.Nonce,
		Tag:       uint8(tx.Tag),
		Payload:   base64.StdEncoding.EncodeToString(tx.Payload),
		Signature: hex.EncodeToString(tx.Signature[:]),
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
		"payload", s.Payload,
		"signature", s.Signature,
	)

	return o, nil
}

type TransactionList []*Transaction

var _ JSONObject = (TransactionList)(nil)

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
