package keystore

import (
	"encoding/hex"

	"errors"

	"golang.org/x/crypto/nacl/secretbox"

	"github.com/mitchellh/mapstructure"
	"github.com/perlin-network/noise/edwards25519"
)

var (
	ErrCouldNotOpenCipher = errors.New("could not open secret box cipher text")
)

// SecretboxParams define the parameters needed
// for using secretbox.
type SecretboxParams struct {
	Nonce string `json:"nonce"`
}

// SecretBoxEncrypt encrypts and private key using the derived key from a KDF.
func (c *Crypto) SecretBoxEncrypt(privateKey edwards25519.PrivateKey, derivedKey []byte) error {
	if c.CipherParams == nil {
		return errors.New("secretbox cipher parameters nil")
		// fail early
	}

	var params SecretboxParams
	err := mapstructure.Decode(c.CipherParams, &params)
	if err != nil {
		return err
	}
	c.CipherParams = params
	nonce, err := hex.DecodeString(params.Nonce)
	if err != nil {
		return err
	}
	if len(nonce) == 24 && len(derivedKey) == 32 {
		var nonceArr [24]byte
		copy(nonceArr[:], nonce)
		var derivedKeyArr [32]byte
		copy(derivedKeyArr[:], derivedKey)
		c.CipherText = hex.EncodeToString(secretbox.Seal(nil, privateKey[:], &nonceArr, &derivedKeyArr))
		return nil
	}
	return errors.New("nonce or derived key were incorrect lengths")
}

// SecretBoxDecrypt decrypts a private key using the provided derived key.
func (c *Crypto) SecretBoxDecrypt(derivedKey []byte) (*edwards25519.PrivateKey, error) {
	if c.CipherParams == nil {
		return nil, errors.New("secretbox cipher parameters nil")
		// fail early
	} else if len(c.CipherText) == 0 {
		return nil, errors.New("secretbox cipher text is not set")
	}
	var params SecretboxParams
	err := mapstructure.Decode(c.CipherParams, &params)
	if err != nil {
		return nil, err
	}
	c.CipherParams = params
	nonce, err := hex.DecodeString(params.Nonce)
	if err != nil {
		return nil, err
	}
	cipherText, err := hex.DecodeString(c.CipherText)
	if err != nil {
		return nil, err
	}

	if len(nonce) == 24 && len(derivedKey) == 32 {
		var nonceArr [24]byte
		copy(nonceArr[:], nonce)
		var derivedKeyArr [32]byte
		copy(derivedKeyArr[:], derivedKey)
		privateKeyBytes, ok := secretbox.Open(nil, cipherText, &nonceArr, &derivedKeyArr)
		if !ok {
			return nil, ErrCouldNotOpenCipher
		}

		var privateKey edwards25519.PrivateKey
		copy(privateKey[:], privateKeyBytes[:edwards25519.SizePrivateKey])
		return &privateKey, nil
	}
	return nil, errors.New("nonce or derived key were incorrect lengths")
}
