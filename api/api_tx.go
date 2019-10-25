package api

import (
	"encoding/base64"
	"encoding/hex"
	"strconv"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type sendTransactionResponse struct {
	// Internal fields.
	ledger *wavelet.Ledger
	tx     *wavelet.Transaction
}

var _ marshalableJSON = (*sendTransactionResponse)(nil)

func (g *Gateway) sendTransaction(ctx *fasthttp.RequestCtx) {
	req := new(sendTransactionRequest)

	if g.Ledger != nil && g.Ledger.TakeSendQuota() == false {
		g.renderError(ctx, ErrInternal(errors.New("rate limit")))
		return
	}

	parser := g.parserPool.Get()
	defer g.parserPool.Put(parser)

	err := req.bind(parser, ctx.PostBody())

	if err != nil {
		g.renderError(ctx, ErrBadRequest(err))
		return
	}

	tx := wavelet.NewSignedTransaction(
		req.sender, req.Nonce, req.Block,
		sys.Tag(req.Tag), req.payload, req.signature,
	)

	// TODO(kenta): check signature and nonce

	if err = g.Ledger.AddTransaction(tx); err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(err, "error adding your transaction to graph")))
		return
	}

	g.render(ctx, &sendTransactionResponse{ledger: g.Ledger, tx: &tx})
}

func (s *sendTransactionResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	if s.ledger == nil || s.tx == nil {
		return nil, errors.New("insufficient parameters were provided")
	}

	o := arena.NewObject()

	o.Set("id", arena.NewString(hex.EncodeToString(s.tx.ID[:])))

	return o.MarshalTo(nil), nil
}

/*
	Get Transaction
*/

type transaction struct {
	// Internal fields.
	tx     *wavelet.Transaction
	status string
}

var _ marshalableJSON = (*transaction)(nil)

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

	// block := g.Ledger.Blocks().Latest()

	res := &transaction{tx: tx}

	// if tx.Depth <= rootDepth {
	// 	res.status = "applied"
	// } else {
	// 	res.status = "received"
	// }

	g.render(ctx, res)
}

func (s *transaction) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	o, err := s.getObject(arena)
	if err != nil {
		return nil, err
	}

	return o.MarshalTo(nil), nil
}

func (s *transaction) getObject(arena *fastjson.Arena) (*fastjson.Value, error) {
	if s.tx == nil {
		return nil, errors.New("insufficient fields specified")
	}

	o := arena.NewObject()

	o.Set("id", arena.NewString(hex.EncodeToString(s.tx.ID[:])))
	o.Set("sender", arena.NewString(hex.EncodeToString(s.tx.Sender[:])))
	o.Set("status", arena.NewString(s.status))
	o.Set("nonce", arena.NewNumberString(strconv.FormatUint(s.tx.Nonce, 10)))
	o.Set("tag", arena.NewNumberInt(int(s.tx.Tag)))
	o.Set("payload", arena.NewString(base64.StdEncoding.EncodeToString(s.tx.Payload)))
	o.Set("signature", arena.NewString(hex.EncodeToString(s.tx.Signature[:])))

	return o, nil
}

type transactionList []*transaction

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

	var transactions transactionList

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

func (s transactionList) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
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
