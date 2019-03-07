package wctl

const (
	SessionInitMessage = "perlin_session_init_"

	RouteSessionInit = "/session/init"
	RouteLedger      = "/ledger"
	RouteAccount     = "/account"
	RouteContract    = "/contract"
	RouteTxList      = "/tx"
	RouteTxSend      = "/tx/send"

	RouteWSBroadcaster  = "/broadcaster"
	RouteWSConsensus    = "/consensus"
	RouteWSStake        = "/stake"
	RouteWSAccounts     = "/accounts"
	RouteWSContracts    = "/contract"
	RouteWSTransactions = "/tx"

	HeaderSessionToken = "X-Session-Token"

	ReqPost = "POST"
	ReqGet  = "GET"
)

type SessionInitRequest struct {
	PublicKey  string `json:"public_key"`
	Signature  string `json:"signature"`
	TimeMillis uint64 `json:"time_millis"`
}

type SessionInitResponse struct {
	Token string `json:"token"`
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

type LedgerStatusResponse struct {
	PublicKey     string   `json:"public_key"`
	HostAddress   string   `json:"address"`
	PeerAddresses []string `json:"peers"`

	RootID     string `json:"root_id"`
	ViewID     uint64 `json:"view_id"`
	Difficulty uint64 `json:"difficulty"`
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

type Account struct {
	PublicKey string `json:"public_key"`
	Balance   uint64 `json:"balance"`
	Stake     uint64 `json:"stake"`

	IsContract bool   `json:"is_contract"`
	NumPages   uint64 `json:"num_mem_pages,omitempty"`
}
