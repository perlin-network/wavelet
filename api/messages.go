package api

const sessionInitSigningPrefix = "perlin_session_init_"

// credentials is the payload sent from clients to server to get a session token
type credentials struct {
	PublicKey  string `json:"PublicKey"     validate:"required,min=60,max=70"`
	TimeMillis int64  `json:"TimeMillis"    validate:"required,gt=0"`
	Sig        string `json:"Sig"           validate:"required,min=120,max=135"`
}

// SessionResponse represents the response from a session call
type SessionResponse struct {
	Token string `json:"token" validate:"required,min=32,max=40"`
}

// Paginate is the payload sent to get from a list call
type Paginate struct {
	Offset *uint64 `json:"offset"`
	Limit  *uint64 `json:"limit"`
}

// SendTransaction is the payload sent to send a transaction
type SendTransaction struct {
	Tag     string `json:"tag"      validate:"required"`
	Payload []byte `json:"payload"  validate:"required"`
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
