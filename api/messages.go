package api

import (
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/wavelet/security"
	"github.com/pkg/errors"
	"time"
)

const sessionInitSigningPrefix = "perlin_session_init_"

type credentials struct {
	TimeMillis int64
	Sig        string
}

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

func (c ClientInfo) String() string {
	return fmt.Sprintf("public_key: %s permissions: %+v", c.PublicKey, c.Permissions)
}

func makeCreds(authKey string) *credentials {
	cred := &credentials{}
	cred.TimeMillis = time.Now().Unix() * 1000
	authStr := fmt.Sprintf("%s%d%d", sessionInitSigningPrefix, cred.TimeMillis, authKey)
	sig := security.Hash([]byte(authStr))
	cred.Sig = hex.EncodeToString(sig)
	return cred
}

func (cred *credentials) validate(authKey string) error {
	if len(authKey) == 0 {
		return errors.New("AuthKey is missing")
	}

	authStr := fmt.Sprintf("%s%d%d", sessionInitSigningPrefix, cred.TimeMillis, authKey)
	sig := security.Hash([]byte(authStr))

	if cred.Sig != hex.EncodeToString(sig) {
		return errors.New("Credentials not valid")
	}
	return nil
}
