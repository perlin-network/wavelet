package wctl

import "github.com/valyala/fastjson"

const (
	SessionInitMessage = "perlin_session_init_"

	RouteSessionInit = "/session/init"
	RouteLedger      = "/ledger"
	RouteAccount     = "/accounts"
	RouteContract    = "/contract"
	RouteTxList      = "/tx"
	RouteTxSend      = "/tx/send"

	RouteWSBroadcaster  = "/broadcaster/poll"
	RouteWSConsensus    = "/consensus/poll"
	RouteWSStake        = "/stake/poll"
	RouteWSAccounts     = "/accounts/poll"
	RouteWSContracts    = "/contract/poll"
	RouteWSTransactions = "/tx/poll"

	HeaderSessionToken = "X-Session-Token"

	ReqPost = "POST"
	ReqGet  = "GET"
)

type FastjsonResponse interface {
	fastjsonUnmarshal([]byte) error
}

type SessionInitRequest struct {
	PublicKey  string `json:"public_key"`
	Signature  string `json:"signature"`
	TimeMillis uint64 `json:"time_millis"`
}

type SessionInitResponse struct {
	Token string `json:"token"`
}

func (s *SessionInitResponse) fastjsonUnmarshal(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	s.Token = string(v.GetStringBytes("token"))

	return nil
}

type SendTransactionRequest struct {
	Sender    string `json:"sender"`
	Tag       byte   `json:"tag"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type SendTransactionResponse struct {
	ID       string   `json:"tx_id"`
	Parents  []string `json:"parent_ids"`
	Critical bool     `json:"is_critical"`
}

func (s *SendTransactionResponse) fastjsonUnmarshal(b []byte) error {
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
	ViewID     uint64 `json:"view_id"`
	Difficulty uint64 `json:"difficulty"`
}

func (l *LedgerStatusResponse) fastjsonUnmarshal(b []byte) error {
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
	l.ViewID = v.GetUint64("view_id")
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

func (t *Transaction) fastjsonUnmarshal(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	t.parse(v)

	return nil
}

func (t *Transaction) parse(v *fastjson.Value) {
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

func (t *TransactionList) fastjsonUnmarshal(b []byte) error {
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
		tx.parse(a[i])

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

func (a *Account) fastjsonUnmarshal(b []byte) error {
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
