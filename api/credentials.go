package api

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"github.com/pkg/errors"
	"time"
)

const sessionInitSigningPrefix = "wavelet_session_init_"

type credentials struct {
	TimeMillis int64
	Sig        string
}

func makeCreds(authKey string) *credentials {
	cred := &credentials{}
	cred.TimeMillis = time.Now().Unix() * 1000
	authStr := fmt.Sprintf("%s%d%s", sessionInitSigningPrefix, cred.TimeMillis, authKey)
	sig := hashBytes([]byte(authStr))
	cred.Sig = hex.EncodeToString(sig)
	return cred
}

func (cred *credentials) validate(authKey string) error {
	if len(authKey) == 0 {
		return errors.New("AuthKey is missing")
	}

	authStr := fmt.Sprintf("%s%d%s", sessionInitSigningPrefix, cred.TimeMillis, authKey)
	sig := hashBytes([]byte(authStr))

	if cred.Sig != hex.EncodeToString(sig) {
		return errors.New("Credentials not valid")
	}
	return nil
}

func hashBytes(bytes []byte) []byte {
	result := sha512.Sum512(bytes)
	// returns [64]byte, use slice to change the type
	return result[:]
}
