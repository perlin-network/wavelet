package api

import (
	"encoding/base64"
	"encoding/hex"
	"strconv"

	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
)

type sendTransactionResponse struct {
	// Internal fields.
	ledger *wavelet.Ledger
	tx     *wavelet.Transaction
}

var _ marshalableJSON = (*sendTransactionResponse)(nil)

func (s *sendTransactionResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	if s.ledger == nil || s.tx == nil {
		return nil, errors.New("insufficient parameters were provided")
	}

	o := arena.NewObject()

	o.Set("id", arena.NewString(hex.EncodeToString(s.tx.ID[:])))

	return o.MarshalTo(nil), nil
}

var _ marshalableJSON = (*transaction)(nil)

type transaction struct {
	// Internal fields.
	tx     *wavelet.Transaction
	status string
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
