package wctl

import (
	"encoding/hex"
	"errors"
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
	Event  string    `json:"event"`
	ID     [32]byte  `json:"tx_id"`
	Sender [32]byte  `json:"sender_id"`
	Depth  uint64    `json:"depth"`
	Tag    byte      `json:"tag"`
	Time   time.Time `json:"time"`
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
func (c *Client) SendTransaction(tag byte, payload []byte) (*TxResponse, error) {
	var res TxResponse

	var nonce [8]byte // TODO(kenta): nonce

	signature := edwards25519.Sign(
		c.PrivateKey,
		append(nonce[:], append([]byte{tag}, payload...)...),
	)

	// TODO: Probably not thread safe, have mutex to guard nonce?

	req := TxRequest{
		Sender:    c.PublicKey,
		Nonce:     c.Nonce + 1,
		Block:     c.Block + 1,
		Tag:       tag,
		Payload:   payload,
		Signature: signature,
	}

	if err := c.RequestJSON(RouteTxSend, ReqPost, &req, &res); err != nil {
		return nil, err
	}

	c.Nonce++
	c.Block++

	return &res, nil
}

// SendTransfer sends a wavelet.Transfer instead of a Payload.
func (c *Client) sendTransfer(tag byte, transfer Marshalable) (*TxResponse, error) {
	return c.SendTransaction(tag, transfer.Marshal())
}

/*
	Structs
*/

type Transaction struct {
	ID        [32]byte `json:"id"`
	Sender    [32]byte `json:"sender"`
	Status    string   `json:"status"`
	Nonce     uint64   `json:"nonce"`
	Depth     uint64   `json:"depth"`
	Tag       byte     `json:"tag"`
	Payload   []byte   `json:"payload"`
	Signature [64]byte `json:"signature"`
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

	t.Status = string(v.GetStringBytes("status"))
	t.Nonce = v.GetUint64("nonce")
	t.Depth = v.GetUint64("depth")
	t.Tag = byte(v.GetUint("tag"))
	t.Payload = v.GetStringBytes("payload")

	if err := jsonHex(v, t.Signature[:], "signature"); err != nil {
		return err
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
	Nonce     uint64   `json:"nonce"`
	Block     uint64   `json:"block"`
	Tag       byte     `json:"tag"`
	Payload   []byte   `json:"payload"`
	Signature [64]byte `json:"signature"`
}

func (s *TxRequest) MarshalJSON() ([]byte, error) {
	var arena fastjson.Arena
	o := arena.NewObject()

	o.Set("sender", arena.NewString(hex.EncodeToString(s.Sender[:])))
	o.Set("nonce", arena.NewNumberInt(int(s.Nonce)))
	o.Set("block", arena.NewNumberInt(int(s.Block)))
	o.Set("tag", arena.NewNumberInt(int(s.Tag)))
	o.Set("payload", arena.NewString(hex.EncodeToString(s.Payload)))
	o.Set("signature", arena.NewString(hex.EncodeToString(s.Signature[:])))

	return o.MarshalTo(nil), nil
}

type TxResponse struct {
	ID [32]byte `json:"id"`
	// Parents  [][32]byte `json:"parent_ids"`
	// Critical bool       `json:"is_critical"`
}

func (s *TxResponse) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	if err := jsonHex(v, s.ID[:], "id"); err != nil {
		return err
	}

	/*
		parentsValue := v.GetArray("parents")
		s.Parents = make([][32]byte, len(parentsValue))

		for i, parent := range parentsValue {
			if _, err := hex.Decode(s.Parents[i][:], parent.MarshalTo(nil)); err != nil {
				return err
			}
		}

		s.Critical = v.GetBool("is_critical")
	*/

	return nil
}
