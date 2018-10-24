package api

import (
	"fmt"
)

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

func (c ClientInfo) String() string {
	return fmt.Sprintf("public_key: %s permissions: %+v", c.PublicKey, c.Permissions)
}
