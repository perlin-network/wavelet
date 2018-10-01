package api

import "fmt"

const sessionInitSigningPrefix = "perlin_session_init_"

type credentials struct {
	PublicKey  string
	TimeMillis int64
	Sig        string
}

// Options represents available options for a local user.
type Options struct {
	ListenAddr string        `toml:"listen_addr"`
	Clients    []*ClientInfo `toml:"clients"`
}

// ClientInfo represents a single clients info.
type ClientInfo struct {
	PublicKey   string            `toml:"public_key"`
	Permissions ClientPermissions `toml:"permissions"`
}

// ClientPermissions represents a single client permissions.
type ClientPermissions struct {
	CanPollTransaction bool `toml:"can_poll_transaction"`
	CanSendTransaction bool `toml:"can_send_transaction"`
	CanControlStats    bool `toml:"can_control_stats"`
}

func (c ClientInfo) String() string {
	return fmt.Sprintf("public_key: %s permissions: %+v", c.PublicKey, c.Permissions)
}
