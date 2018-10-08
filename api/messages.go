package api

import (
	"fmt"
)

const sessionInitSigningPrefix = "wavelet_session_init_"

// Options represents available options for a local user.
type Options struct {
	ListenAddr string
	Clients    []*ClientInfo
}

// ClientInfo represents a single clients info.
type ClientInfo struct {
	PublicKey   string
	AuthKey     string
	Permissions ClientPermissions
}

// ClientPermissions represents a single client permissions.
type ClientPermissions struct {
	CanPollTransaction bool
	CanSendTransaction bool
	CanControlStats    bool
}

type ServerVersion struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
}

func (c ClientInfo) String() string {
	return fmt.Sprintf("public_key: %s permissions: %+v", c.PublicKey, c.Permissions)
}
