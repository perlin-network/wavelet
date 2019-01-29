package api

//------------------------
// Request Payloads
//------------------------

// CredentialsRequest is the payload sent from clients to server to get a session token
type CredentialsRequest struct {
	PublicKey  string `json:"public_key"     validate:"required,len=64"`
	TimeMillis int64  `json:"time_millis"    validate:"required,gt=0"`
	Sig        string `json:"signature"      validate:"required,len=128"`
}

// ListTransactionsRequest retrieves paginated transactions based on a specified tag
type ListTransactionsRequest struct {
	Tag    *uint32 `json:"tag"      validate:"omitempty,max=30"`
	Offset *uint64 `json:"offset"`
	Limit  *uint64 `json:"limit"    validate:"omitempty,max=1024"`
}

// SendTransactionRequest is the payload sent to send a transaction
type SendTransactionRequest struct {
	Tag     uint32 `json:"tag"      validate:"required,max=30"`
	Payload []byte `json:"payload"  validate:"required,max=1024"`
}

// GetContractRequest is the payload request to get a smart contract
type GetContractRequest struct {
	TransactionID string `json:"transaction_id" validate:"required,min=32,max=96"`
}

// ListContractsRequest retrieves paginated contracts
type ListContractsRequest struct {
	Offset *uint64 `json:"offset"`
	Limit  *uint64 `json:"limit"    validate:"omitempty,max=1024"`
}

// ExecuteContractRequest executes a contract locally.
type ExecuteContractRequest struct {
	ContractID string `json:"contract_id" validate:"required,len=64"`
	Entry      string `json:"entry_id" validate:"required,max=255"`
	Param      []byte `json:"param"`
}

type ForwareTransactionRequest struct {

	wired := &wire.Transaction{
		Sender:  b.Wallet.PublicKey,
		Nonce:   nonce,
		Parents: parents,
		Tag:     tag,
		Payload: payload,
	}

	Sender               []byte   `protobuf:"bytes,1,opt,name=sender,proto3" json:"sender,omitempty"`
	Nonce                uint64   `protobuf:"varint,2,opt,name=nonce,proto3" json:"nonce,omitempty"`
	Parents              [][]byte `protobuf:"bytes,3,rep,name=parents" json:"parents,omitempty"`
	Tag                  uint32   `protobuf:"varint,4,opt,name=tag,proto3" json:"tag,omitempty"`
	Payload              []byte   `protobuf:"bytes,5,opt,name=payload,proto3" json:"payload,omitempty"`
	Signature            []byte   `protobuf:"bytes,6,opt,name=signature,proto3" json:"signature,omitempty"`
	Timestamp            int64    `protobuf:"varint,7,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
}

//------------------------
// Response Payloads
//------------------------

// ErrorResponse is a payload when there is an error
type ErrorResponse struct {
	StatusCode int         `json:"status"`
	Error      interface{} `json:"error,omitempty"`
}

// SessionResponse represents the response from a session call
type SessionResponse struct {
	Token string `json:"token"`
}

// ServerVersion represents the response from a server version call
type ServerVersion struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	OSArch    string `json:"os_arch"`
}

// LedgerState represents the state of the ledger.
type LedgerState struct {
	PublicKey string                 `json:"public_key"`
	Address   string                 `json:"address"`
	Peers     []string               `json:"peers"`
	State     map[string]interface{} `json:"state"`
}

// TransactionResponse represents the response from a sent transaction
type TransactionResponse struct {
	TransactionID string `json:"transaction_id,omitempty"`
	Code          []byte `json:"code,omitempty"`
}

type ExecuteContractResponse struct {
	Result []byte `json:"result"`
}

type FindParentsResponse struct {
	ParentIDs []string `json:"parents"`
}
