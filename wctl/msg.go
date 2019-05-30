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

package wctl

import (
	"github.com/valyala/fastjson"
)

const (
	RouteLedger   = "/ledger"
	RouteAccount  = "/accounts"
	RouteContract = "/contract"
	RouteTxList   = "/tx"
	RouteTxSend   = "/tx/send"

	RouteWSBroadcaster  = "/poll/broadcaster"
	RouteWSConsensus    = "/poll/consensus"
	RouteWSStake        = "/poll/stake"
	RouteWSAccounts     = "/poll/accounts"
	RouteWSContracts    = "/poll/contract"
	RouteWSTransactions = "/poll/tx"
	RouteWSMetrics      = "/poll/metrics"

	ReqPost = "POST"
	ReqGet  = "GET"
)

var (
	_ UnmarshalableJSON = (*SendTransactionResponse)(nil)
	_ UnmarshalableJSON = (*LedgerStatusResponse)(nil)
	_ UnmarshalableJSON = (*Transaction)(nil)
	_ UnmarshalableJSON = (*TransactionList)(nil)
	_ UnmarshalableJSON = (*Account)(nil)

	_ MarshalableJSON = (*SendTransactionRequest)(nil)
)

type UnmarshalableJSON interface {
	UnmarshalJSON([]byte) error
}

type MarshalableJSON interface {
	MarshalJSON() ([]byte, error)
}

type SendTransactionRequest struct {
	Sender    string `json:"sender"`
	Tag       byte   `json:"tag"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func (s *SendTransactionRequest) MarshalJSON() ([]byte, error) {
	var arena fastjson.Arena
	o := arena.NewObject()

	o.Set("sender", arena.NewString(s.Sender))
	o.Set("tag", arena.NewNumberInt(int(s.Tag)))
	o.Set("payload", arena.NewString(s.Payload))
	o.Set("signature", arena.NewString(s.Signature))

	return o.MarshalTo(nil), nil
}

type SendTransactionResponse struct {
	ID       string   `json:"tx_id"`
	Parents  []string `json:"parent_ids"`
	Critical bool     `json:"is_critical"`
}

func (s *SendTransactionResponse) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	s.ID = string(v.GetStringBytes("tx_id"))

	parentsValue := v.GetArray("parent_ids")
	for _, parent := range parentsValue {
		s.Parents = append(s.Parents, parent.String())
	}

	s.Critical = v.GetBool("is_critical")

	return nil
}

type LedgerStatusResponse struct {
	PublicKey     string   `json:"public_key"`
	HostAddress   string   `json:"address"`
	PeerAddresses []string `json:"peers"`

	RootID     string `json:"root_id"`
	RoundID    uint64 `json:"round_id"`
	Difficulty uint64 `json:"difficulty"`
}

func (l *LedgerStatusResponse) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	l.PublicKey = string(v.GetStringBytes("public_key"))
	l.HostAddress = string(v.GetStringBytes("address"))

	peerValue := v.GetArray("peers")
	for _, peer := range peerValue {
		l.PeerAddresses = append(l.PeerAddresses, peer.String())
	}

	l.RootID = string(v.GetStringBytes("root_id"))
	l.RoundID = v.GetUint64("round_id")
	l.Difficulty = v.GetUint64("difficulty")

	return nil
}

type Transaction struct {
	ID string `json:"id"`

	Sender  string `json:"sender"`
	Creator string `json:"creator"`

	Parents []string `json:"parents"`

	Timestamp uint64 `json:"timestamp"`

	Tag     byte   `json:"tag"`
	Payload []byte `json:"payload"`

	AccountsMerkleRoot string `json:"accounts_root"`

	SenderSignature  string `json:"sender_signature"`
	CreatorSignature string `json:"creator_signature"`

	Depth uint64 `json:"depth"`
}

func (t *Transaction) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	t.ParseJSON(v)

	return nil
}

func (t *Transaction) ParseJSON(v *fastjson.Value) {
	t.ID = string(v.GetStringBytes("id"))
	t.Sender = string(v.GetStringBytes("sender"))
	t.Creator = string(v.GetStringBytes("creator"))

	parentsValue := v.GetArray("parents")
	for _, parent := range parentsValue {
		t.Parents = append(t.Parents, parent.String())
	}

	t.Timestamp = v.GetUint64("timestamp")
	t.Tag = byte(v.GetUint("tag"))
	t.Payload = v.GetStringBytes("payload")
	t.AccountsMerkleRoot = string(v.GetStringBytes("accounts_root"))
	t.SenderSignature = string(v.GetStringBytes("sender_signature"))
	t.CreatorSignature = string(v.GetStringBytes("creator_signature"))
	t.Depth = v.GetUint64("depth")
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

	var tx *Transaction
	for i := range a {
		tx = &Transaction{}
		tx.ParseJSON(a[i])

		list = append(list, *tx)
	}

	*t = list

	return nil
}

type Account struct {
	PublicKey string `json:"public_key"`
	Balance   uint64 `json:"balance"`
	Stake     uint64 `json:"stake"`

	IsContract bool   `json:"is_contract"`
	NumPages   uint64 `json:"num_mem_pages,omitempty"`
}

func (a *Account) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	a.PublicKey = string(v.GetStringBytes("public_key"))
	a.Balance = v.GetUint64("balance")
	a.Stake = v.GetUint64("stake")
	a.IsContract = v.GetBool("is_contract")
	a.NumPages = v.GetUint64("num_mem_pages")

	return nil
}
