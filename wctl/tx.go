package wctl

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/valyala/fastjson"
)

var (
	_ UnmarshalableJSON = (*TxResponse)(nil)
	_ UnmarshalableJSON = (*Transaction)(nil)
	_ UnmarshalableJSON = (*TransactionList)(nil)
	_ MarshalableJSON   = (*TxRequest)(nil)
)

var (
	// ErrInsufficientPerls is returned when you don't have enough PERLs.
	ErrInsufficientPerls = errors.New("Insufficient PERLs")
)

type TransactionEvent struct {
	Event   string    `json:"event"`
	ID      [32]byte  `json:"tx_id"`
	Sender  [32]byte  `json:"sender_id"`
	Creator [32]byte  `json:"creator_id"`
	Depth   uint64    `json:"depth"`
	Tag     byte      `json:"tag"`
	Time    time.Time `json:"time"`
}

type Transaction struct {
	ID      [32]byte `json:"id"`
	Sender  [32]byte `json:"sender"`
	Creator [32]byte `json:"creator"`
	Status  string   `json:"status"`
	Nonce   uint64   `json:"nonce"`
	Depth   uint64   `json:"depth"`
	Tag     byte     `json:"tag"`
	Payload []byte   `json:"payload"`

	Seed    [32]byte `json:"seed"`
	SeedLen uint8    `json"seed_len"`

	SenderSignature  [64]byte `json:"sender_signature"`
	CreatorSignature [64]byte `json:"creator_signature"`

	Parents [][32]byte `json:"parents"`
}

// ListTransactions calls the /tx endpoint of the API to list all transactions.
// The arguments are optional, zero values would default them.
func (c *Client) ListTransactions(senderID string, creatorID string, offset uint64, limit uint64) ([]Transaction, error) {
	vals := url.Values{}

	if senderID != "" {
		vals.Set("sender", string(senderID[:]))
	}

	if creatorID != "" {
		vals.Set("creator", string(creatorID[:]))
	}

	if offset != 0 {
		vals.Set("offset", strconv.FormatUint(offset, 10))
	}

	if limit != 0 {
		vals.Set("limit", strconv.FormatUint(limit, 10))
	}

	path := RouteTxList + "?" + vals.Encode()

	var res TransactionList
	if err := c.RequestJSON(path, ReqGet, nil, &res); err != nil {
		return nil, err
	}

	return res, nil
}

// GetTransaction calls the /tx endpoint to query a single transaction.
func (c *Client) GetTransaction(txID [32]byte) (*Transaction, error) {
	path := RouteTxList + "/" + hex.EncodeToString(txID[:])

	var res Transaction
	if err := c.RequestJSON(path, ReqGet, nil, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

// SendTransaction calls the /tx/send endpoint to send a raw payload.
// Payloads are best crafted with wavelet.Transfer.
func (c *Client) sendTransaction(tag byte, payload []byte) (*TxResponse, error) {
	var res TxResponse

	var nonce [8]byte // TODO(kenta): nonce

	signature := edwards25519.Sign(
		c.PrivateKey,
		append(nonce[:], append([]byte{tag}, payload...)...),
	)

	req := TxRequest{
		Sender:    c.PublicKey,
		Tag:       tag,
		Payload:   payload,
		Signature: signature,
	}

	if err := c.RequestJSON(RouteTxSend, ReqPost, &req, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

// SendTransfer sends a wavelet.Transfer instead of a Payload.
func (c *Client) sendTransfer(tag byte, transfer Marshalable) (*TxResponse, error) {
	return c.sendTransaction(tag, transfer.Marshal())
}

// PollTransactions calls the callback for each WS event received.
func (c *Client) PollTransactions(callback func(txs []TransactionEvent),
	txID string, senderID string, creatorID string, tag *byte) (func(), error) {

	v := url.Values{}

	if txID != "" {
		v.Set("tx_id", txID)
	}

	if senderID != "" {
		v.Set("sender", senderID)
	}

	if creatorID != "" {
		v.Set("creator", creatorID)
	}

	if tag != nil {
		v.Set("tag", fmt.Sprintf("%x", *tag))
	}

	return c.pollWS(func(b []byte) {
		var parser fastjson.Parser
		v, err := parser.ParseBytes(b)
		if err != nil {
			return
		}

		a := v.GetArray()
		txs := make([]TransactionEvent, 0, len(a))

		for _, o := range a {
			var t TransactionEvent

			if err := jsonHex(o, t.ID[:], "tx_id"); err != nil {
				continue
			}

			if err := jsonHex(o, t.Sender[:], "sender_id"); err != nil {
				continue
			}

			if err := jsonHex(o, t.Creator[:], "creator_id"); err != nil {
				fmt.Println("err in tx.go", err)
				continue
			}

			t.Event = string(o.GetStringBytes("event"))
			t.Depth = o.GetUint64("depth")
			t.Tag = byte(o.GetUint("tag"))

			Time, err := time.Parse(
				time.RFC3339, string(o.GetStringBytes("time")),
			)

			if err != nil {
				continue
			}

			t.Time = Time

			txs = append(txs, t)
		}

		callback(txs)
	}, RouteWSTransactions, v)
}

func (t *Transaction) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	return t.ParseJSON(v)
}

func (t *Transaction) ParseJSON(v *fastjson.Value) error {
	if err := jsonHex(v, t.ID[:], "id"); err != nil {
		return err
	}

	if err := jsonHex(v, t.Sender[:], "sender"); err != nil {
		return err
	}

	if err := jsonHex(v, t.Creator[:], "creator"); err != nil {
		return err
	}

	t.Status = string(v.GetStringBytes("status"))
	t.Nonce = v.GetUint64("nonce")
	t.Depth = v.GetUint64("depth")
	t.Tag = byte(v.GetUint("tag"))
	t.Payload = v.GetStringBytes("payload")

	if err := jsonHex(v, t.Seed[:], "seed"); err != nil {
		return err
	}

	t.SeedLen = uint8(v.GetUint("seed_len"))

	if err := jsonHex(v, t.SenderSignature[:], "sender_signature"); err != nil {
		return err
	}

	if err := jsonHex(v, t.CreatorSignature[:], "creator_signature"); err != nil {
		return err
	}

	parentsValue := v.GetArray("parents")
	t.Parents = make([][32]byte, len(parentsValue))

	for i, parent := range parentsValue {
		if _, err := hex.Decode(t.Parents[i][:], parent.MarshalTo(nil)); err != nil {
			return err
		}
	}

	return nil
}

type TransactionList []Transaction

func (t *TransactionList) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	a, err := v.Array()
	if err != nil {
		return err
	}

	var list []Transaction

	for _, v := range a {
		tx := &Transaction{}
		tx.ParseJSON(v)

		list = append(list, *tx)
	}

	*t = list

	return nil
}

type TxRequest struct {
	Sender    [32]byte `json:"sender"`
	Tag       byte     `json:"tag"`
	Payload   []byte   `json:"payload"`
	Signature [64]byte `json:"signature"`
}

func (s *TxRequest) MarshalJSON() ([]byte, error) {
	var arena fastjson.Arena
	o := arena.NewObject()

	o.Set("sender", arena.NewString(hex.EncodeToString(s.Sender[:])))
	o.Set("tag", arena.NewNumberInt(int(s.Tag)))
	o.Set("payload", arena.NewString(hex.EncodeToString(s.Payload)))
	o.Set("signature", arena.NewString(hex.EncodeToString(s.Signature[:])))

	return o.MarshalTo(nil), nil
}

type TxResponse struct {
	ID       [32]byte   `json:"tx_id"`
	Parents  [][32]byte `json:"parent_ids"`
	Critical bool       `json:"is_critical"`
}

func (s *TxResponse) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	if err := jsonHex(v, s.ID[:], "tx_id"); err != nil {
		return err
	}

	parentsValue := v.GetArray("parents")
	s.Parents = make([][32]byte, len(parentsValue))

	for i, parent := range parentsValue {
		if _, err := hex.Decode(s.Parents[i][:], parent.MarshalTo(nil)); err != nil {
			return err
		}
	}

	s.Critical = v.GetBool("is_critical")

	return nil
}
