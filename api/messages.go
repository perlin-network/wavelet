package api

const sessionInitSigningPrefix = "perlin_session_init_"

type credentials struct {
	PublicKey  string `json:"PublicKey" validate:"required"`
	TimeMillis int64  `json:"TimeMillis" validate:"required,gte=0"`
	Sig        string `json:"Sig" validate:"required"`
}

// SessionResponse represents the response from a session call
type SessionResponse struct {
	Token string `json:"token" validate:"required"`
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
