package api

import (
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/wavelet/security"
	"github.com/pkg/errors"
	"time"
)

type credentials struct {
	TimeMillis int64
	Sig        string
}

func makeCreds(authKey string) *credentials {
	cred := &credentials{}
	cred.TimeMillis = time.Now().Unix() * 1000
	authStr := fmt.Sprintf("%s%d%s", sessionInitSigningPrefix, cred.TimeMillis, authKey)
	sig := security.Hash([]byte(authStr))
	cred.Sig = hex.EncodeToString(sig)
	return cred
}

func (cred *credentials) validate(authKey string) error {
	if len(authKey) == 0 {
		return errors.New("AuthKey is missing")
	}

	authStr := fmt.Sprintf("%s%d%s", sessionInitSigningPrefix, cred.TimeMillis, authKey)
	sig := security.Hash([]byte(authStr))

	if cred.Sig != hex.EncodeToString(sig) {
		return errors.New("Credentials not valid")
	}
	return nil
}
