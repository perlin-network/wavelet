package api

import (
	"fmt"
)

const sessionInitSigningPrefix = "perlin_session_init_"

type credentials struct {
	PublicKey  string `json:"PublicKey"`
	TimeMillis int64  `json:"TimeMillis"`
	Sig        string `json:"Sig"`
}

// Options represents available options for a local user.
type Options struct {
	ListenAddr string
	Clients    []*ClientInfo
}

// ClientInfo represents a single clients info.
type ClientInfo struct {
	PublicKey   string
	Permissions ClientPermissions
}

// ClientPermissions represents a single client permissions.
type ClientPermissions struct {
	CanPollTransaction bool
	CanSendTransaction bool
	CanControlStats    bool
}

// SessionResponse represents the response from a session call
type SessionResponse struct {
	Token string `json:"token"`
}

// ServerVersion represents the response from a server version call
type ServerVersion struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
}

func (c ClientInfo) String() string {
	return fmt.Sprintf("public_key: %s permissions: %+v", c.PublicKey, c.Permissions)
}
