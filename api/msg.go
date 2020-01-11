// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package api

import (
	"net/http"

	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

/*
	Message responses
*/

type MsgResponse struct {
	Message string `json:"msg"`
}

var _ log.JSONObject = (*MsgResponse)(nil)

func (s *MsgResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"msg", s.Message)
}

func (s *MsgResponse) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueAny(v, &s.Message, "msg")
}

func (s *MsgResponse) MarshalEvent(ev *zerolog.Event) {
	ev.Msg(s.Message)
}

type ErrResponse struct {
	Error          string `json:"error,omitempty"` // low-level runtime error
	HTTPStatusCode int    `json:"status"`          // http response status code
}

var _ log.JSONObject = (*ErrResponse)(nil)

func (e *ErrResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"status", http.StatusText(e.HTTPStatusCode),
		"error,omitempty", e.Error,
	)
}

func (e *ErrResponse) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueBatch(v,
		"error", &e.Error,
		"status", &e.HTTPStatusCode)
}

<<<<<<< HEAD
func (e *ErrResponse) MarshalEvent(ev *zerolog.Event) {
	ev.Err(errors.New(e.Error))
	ev.Int("code", e.HTTPStatusCode)
	ev.Msg(e.Error)
=======
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
		preferredBlock := preferred.Value().(*wavelet.Block)

		preferredObj := arena.NewObject()
		preferredObj.Set("merkle_root",
			arena.NewString(hex.EncodeToString(preferredBlock.Merkle[:])))
		preferredObj.Set("height",
			arena.NewNumberString(strconv.FormatUint(preferredBlock.Index, 10)))
		preferredObj.Set("id",
			arena.NewString(hex.EncodeToString(preferredBlock.ID[:])))
		preferredObj.Set("transactions",
			arena.NewNumberInt(len(preferredBlock.Transactions)))

		o.Set("preferred", preferredObj)
	} else {
		o.Set("preferred", arena.NewNull())
	}

	o.Set("num_missing_tx", arena.NewNumberInt(s.ledger.Transactions().MissingLen()))

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
	o.Set("height", arena.NewNumberString(strconv.FormatUint(s.tx.Block, 10)))
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

type account struct {
	// Internal fields.
	id     wavelet.AccountID
	ledger *wavelet.Ledger

	balance    uint64
	gasBalance uint64
	stake      uint64
	reward     uint64
	isContract bool
	numPages   uint64
}

func (s *account) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	if s.ledger == nil || s.id == wavelet.ZeroAccountID {
		return nil, errors.New("insufficient fields specified")
	}

	o := arena.NewObject()

	o.Set("public_key", arena.NewString(hex.EncodeToString(s.id[:])))
	o.Set("balance", arena.NewNumberString(strconv.FormatUint(s.balance, 10)))
	o.Set("gas_balance", arena.NewNumberString(strconv.FormatUint(s.gasBalance, 10)))
	o.Set("stake", arena.NewNumberString(strconv.FormatUint(s.stake, 10)))
	o.Set("reward", arena.NewNumberString(strconv.FormatUint(s.reward, 10)))

	if s.isContract {
		o.Set("is_contract", arena.NewTrue())
	} else {
		o.Set("is_contract", arena.NewFalse())
	}

	if s.numPages != 0 {
		o.Set("num_mem_pages", arena.NewNumberString(strconv.FormatUint(s.numPages, 10)))
	}

	return o.MarshalTo(nil), nil
}

type errResponse struct {
	Err            error `json:"-"` // low-level runtime error
	HTTPStatusCode int   `json:"-"` // http response status code
}

func (e *errResponse) marshalJSON(arena *fastjson.Arena) []byte {
	o := arena.NewObject()

	o.Set("status", arena.NewString(http.StatusText(e.HTTPStatusCode)))

	if e.Err != nil {
		o.Set("error", arena.NewString(e.Err.Error()))
	}

	return o.MarshalTo(nil)
>>>>>>> f59047e31aa71fc9fbecf1364e37a5e5641f8e01
}

func ErrBadRequest(err error) *ErrResponse {
	return &ErrResponse{
		Error:          err.Error(),
		HTTPStatusCode: http.StatusBadRequest,
	}
}

func ErrNotFound(err error) *ErrResponse {
	return &ErrResponse{
		Error:          err.Error(),
		HTTPStatusCode: http.StatusNotFound,
	}
}

func ErrInternal(err error) *ErrResponse {
	return &ErrResponse{
		Error:          err.Error(),
		HTTPStatusCode: http.StatusInternalServerError,
	}
}
