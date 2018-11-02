package api

//------------------------
// Request Payloads
//------------------------

// CredentialsRequest is the payload sent from clients to server to get a session token
type CredentialsRequest struct {
	PublicKey  string `json:"PublicKey"     validate:"required,len=64"`
	TimeMillis int64  `json:"TimeMillis"    validate:"required,gt=0"`
	Sig        string `json:"Sig"           validate:"required,len=128"`
}

// ListTransactionsRequest retrieves paginated transactions based on a specified tag
type ListTransactionsRequest struct {
	Tag    *string `json:"tag"      validate:"omitempty,max=30"`
	Offset *uint64 `json:"offset"`
	Limit  *uint64 `json:"limit"    validate:"omitempty,max=1024"`
}

// SendTransactionRequest is the payload sent to send a transaction
type SendTransactionRequest struct {
	Tag     string `json:"tag"      validate:"required,max=30"`
	Payload []byte `json:"payload"  validate:"required,max=1024"`
}

// GetContractRequest is the payload request to get a smart contract
type GetContractRequest struct {
	TransactionID string `json:"transaction_id"   validate:"required,len=64"`
}

// ListContractsRequest retrieves paginated contracts
type ListContractsRequest struct {
	Offset *uint64 `json:"offset"`
	Limit  *uint64 `json:"limit"    validate:"omitempty,max=1024"`
}

//------------------------
// Response Payloads
//------------------------

// ErrorResponse is a payload when there is an error
type ErrorResponse struct {
	StatusCode int         `json:"status"`
	Error      interface{} `json:"error"`
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
}
