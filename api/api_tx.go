package api

import (
	"encoding/base64"
	"encoding/hex"
	"strconv"

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

func (s *TxResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	log.ObjectAny(arena, o, "id", s.ID[:])

	return o.MarshalTo(nil), nil
}

func (s *TxResponse) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("id", s.ID[:])
	ev.Msg("Transaction sent.")
}

func (s *TxResponse) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueHex(v, s.ID[:], "id")
}

/*
	Get Transaction
*/

type Transaction struct {
	ID        wavelet.TransactionID `json:"id"`
	Sender    wavelet.AccountID     `json:"sender"`
	Status    string                `json:"status"`
	Nonce     uint64                `json:"nonce"`
	Block     uint64                `json:"height"`
	Tag       sys.Tag               `json:"tag"`
	Payload   []byte                `json:"payload"` // base64
	Signature wavelet.Signature     `json:"signature"`
}

var _ log.JSONObject = (*Transaction)(nil)

func convertTransaction(tx *wavelet.Transaction, status string) Transaction {
	return Transaction{
		ID:        tx.ID,
		Sender:    tx.Sender,
		Status:    status,
		Nonce:     tx.Nonce,
		Block:     tx.Block,
		Tag:       tx.Tag,
		Payload:   tx.Payload,
		Signature: tx.Signature,
	}
}

const (
	StatusApplied  = "applied"
	StatusReceived = "received"
)

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

	var converted = convertTransaction(tx, "")
	if tx.Block < g.Ledger.Blocks().Latest().Index {
		converted.Status = StatusApplied
	} else {
		converted.Status = StatusReceived
	}

	g.render(ctx, &converted)
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

	err := log.ObjectBatch(arena, o,
		"id", s.ID[:],
		"sender", s.Sender,
		"nonce", s.Nonce,
		"tag", s.Tag,
		"payload", base64.StdEncoding.EncodeToString(s.Payload),
		"signature", s.Signature,
	)

	return o, err
}

func (s *Transaction) UnmarshalValue(v *fastjson.Value) error {
	log.ValueHex(v, s.ID[:], "id")
	log.ValueHex(v, s.Sender, "sender")
	log.ValueHex(v, s.Signature, "signature")

	s.Nonce = v.GetUint64("nonce")
	s.Tag = sys.Tag(v.GetUint("tag"))

	pl, _ := log.ValueBase64(v, "payload")
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
	ev.Uint8("tag", uint8(s.Tag))

	ev.Int("payload_len", len(s.Payload))
}

func (s *Transaction) MarshalZerologObject(ev *zerolog.Event) {
	s.MarshalEvent(ev)
}

type TransactionList []Transaction

var _ log.JSONObject = (*TransactionList)(nil)

func (g *Gateway) listTransactions(ctx *fasthttp.RequestCtx) {
	var (
		sender        wavelet.AccountID
		offset, limit uint64
		err           error
	)

	queryArgs := ctx.QueryArgs()
	if raw := string(queryArgs.Peek("sender")); len(raw) > 0 {
		slice, err := hex.DecodeString(raw)
		if err != nil {
			g.renderError(ctx, ErrBadRequest(errors.Wrap(
				err, "sender ID must be presented as valid hex")))
			return
		}

		if len(slice) != wavelet.SizeAccountID {
			g.renderError(ctx, ErrBadRequest(errors.Errorf(
				"sender ID must be %d bytes long", wavelet.SizeAccountID)))
			return
		}

		copy(sender[:], slice)
	}

	if raw := string(queryArgs.Peek("offset")); len(raw) > 0 {
		offset, err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			g.renderError(ctx, ErrBadRequest(errors.Wrap(
				err, "could not parse offset")))
			return
		}
	}

	if raw := string(queryArgs.Peek("limit")); len(raw) > 0 {
		limit, err = strconv.ParseUint(raw, 10, 64)

		if err != nil {
			g.renderError(ctx, ErrBadRequest(errors.Wrap(
				err, "could not parse limit")))
			return
		}
	}

	if limit > maxPaginationLimit {
		limit = maxPaginationLimit
	}

	var transactions TransactionList
	var latestBlockIndex = g.Ledger.Blocks().Latest().Index

	// TODO: maybe there is be a better way to do this? Currently, this iterates
	// the entire transaction list
	g.Ledger.Transactions().Iterate(func(tx *wavelet.Transaction) bool {
		if tx.Block < offset {
			return true
		}
		if uint64(len(transactions)) >= limit {
			return true
		}
		if sender != wavelet.ZeroAccountID && tx.Sender != sender {
			return true
		}

		var status = StatusReceived
		if tx.Block <= latestBlockIndex {
			status = StatusApplied
		}

		transactions = append(transactions, convertTransaction(tx, status))
		return true
	})

	g.render(ctx, &transactions)
}

func (s *TransactionList) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	list := arena.NewArray()

	for i, v := range *s {
		o, err := v.getObject(arena)
		if err != nil {
			return nil, err
		}

		list.SetArrayItem(i, o)
	}

	return list.MarshalTo(nil), nil
}

func (s *TransactionList) UnmarshalValue(v *fastjson.Value) error {
	a, err := v.Array()
	if err != nil {
		return log.NewErrUnmarshalErr(v, nil, err)
	}

	var txs = make([]Transaction, len(a))

	for i, v := range a {
		var t Transaction

		if err := t.UnmarshalValue(v); err != nil {
			return errors.Wrap(err, "Failed to unmarshal tx index "+
				strconv.Itoa(i))
		}

		txs[i] = t
	}

	return nil
}

// MarshalEvent doesn't return the full contents of Payload, but only its
// length.
func (s *TransactionList) MarshalEvent(ev *zerolog.Event) {
	var array = zerolog.Arr()
	for _, tx := range *s {
		array.Object(&tx)
	}

	ev.Array("txs", array)
}
