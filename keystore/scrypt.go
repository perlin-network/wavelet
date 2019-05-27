package keystore

import (
	"encoding/hex"
	"errors"

	"github.com/mitchellh/mapstructure"
	"golang.org/x/crypto/scrypt"
)

// ScryptParams define the neccessary parameters for using
// the scrypt KDF.
type ScryptParams struct {
	N      int    `json:"n"`
	P      int    `json:"p"`
	R      int    `json:"r"`
	KeyLen int    `json:"keyLen"`
	Salt   string `json:"salt"`
}

// ScryptKeyFromPassword takes a password and the crypto parameters.
// Returns the derived key from the scrypt KDF.
func (c *Crypto) ScryptKeyFromPassword(password string) ([]byte, error) {
	if c.KDFParams == nil {
		return nil, errors.New("scrypt KDF parameters are nil")
		// fail early
	}
	var params ScryptParams
	err := mapstructure.Decode(c.KDFParams, &params)
	if err != nil {
		return nil, err
	}
	c.KDFParams = params

	salt, err := hex.DecodeString(params.Salt)
	if err != nil {
		return nil, err
	}
	derivedKey, err := scrypt.Key([]byte(password), salt, params.N, params.R, params.P, params.KeyLen)
	if err != nil {
		return nil, err
	}
	return derivedKey, nil
}
