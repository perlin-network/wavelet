package keystore

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"time"

	"github.com/perlin-network/noise/edwards25519"
)

// EncryptedKey contains details about the Cipher / KDF parameters,
// the encrypted key (cipher text), and account details for easy
// tracking.
type EncryptedKey struct {
	Account     string    `json:"account"`
	Version     int       `json:"version"`
	TimeCreated time.Time `json:"created"`
	Crypto      Crypto    `json:"crypto"`
}

// NewEncryptedKey takes a private key and a password and returns an encrpyted key struct with
// the default parameters.
func NewEncryptedKey(privateKey edwards25519.PrivateKey, password string) (*EncryptedKey, error) {
	publicKey := privateKey.Public()
	accountID := hex.EncodeToString(publicKey[:])
	ek := &EncryptedKey{
		Account:     accountID,
		Version:     Version,
		TimeCreated: time.Now(),
		Crypto:      Crypto{},
	}
	err := ek.Crypto.SetDefaultCipher()
	if err != nil {
		return nil, err
	}
	err = ek.Crypto.SetDefaultKDFParams()
	if err != nil {
		return nil, err
	}

	dk, err := ek.Crypto.DeriveKeyFromPassword(password)
	if err != nil {
		return nil, err
	}

	err = ek.Crypto.EncryptPrivateKey(privateKey, dk)
	if err != nil {
		return nil, err
	}
	return ek, nil
}

// NewEncryptedKeyWithOptions overrides the default encryption options. Takes a private key, password, and options.
func NewEncryptedKeyWithOptions(privateKey edwards25519.PrivateKey, password string, opts CryptoOptions) (*EncryptedKey, error) {
	publicKey := privateKey.Public()
	accountID := hex.EncodeToString(publicKey[:])
	ek := &EncryptedKey{
		Account:     accountID,
		Version:     Version,
		TimeCreated: time.Now(),
		Crypto:      Crypto{},
	}
	ek.Crypto.CipherParams = opts.CipherParams
	ek.Crypto.KDFParams = opts.KDFParams
	ek.Crypto.Cipher = opts.Cipher
	ek.Crypto.KDF = opts.KDF

	dk, err := ek.Crypto.DeriveKeyFromPassword(password)
	if err != nil {
		return nil, err
	}

	err = ek.Crypto.EncryptPrivateKey(privateKey, dk)
	if err != nil {
		return nil, err
	}
	return ek, nil
}

// Decrypt is a helper function that derives the cipher key using the kdf function specified
// and the private key. Returns the private key.
func (e *EncryptedKey) Decrypt(password string) (*edwards25519.PrivateKey, error) {
	dk, err := e.Crypto.DeriveKeyFromPassword(password)
	if err != nil {
		return nil, err
	}
	privKey, err := e.Crypto.DecryptPrivateKey(dk)
	if err != nil {
		return nil, err
	}
	return privKey, nil
}

// EncryptPrivateKey is a helper function that handles the encryption of the private key with the
// derived key produced by the KDF.
func (c *Crypto) EncryptPrivateKey(privateKey edwards25519.PrivateKey, derivedKey []byte) error {
	switch c.Cipher {
	// secretbox
	case DefaultCipher:
		return c.SecretBoxEncrypt(privateKey, derivedKey)
	}
	return errors.New("unsupported cipher (secretbox only)")
	// CHANGE THIS IF YOU ADD NEW Ciphers ^^^^^^^^^^^^^^^^^^
}

// DecryptPrivateKey takes a derived key from the KDF and uses that to unlock the cipher.
func (c *Crypto) DecryptPrivateKey(derivedKey []byte) (*edwards25519.PrivateKey, error) {
	switch c.Cipher {
	// secretbox
	case DefaultCipher:
		return c.SecretBoxDecrypt(derivedKey)
	}
	return nil, errors.New("unsupported cipher (secretbox only)")
	// CHANGE THIS IF YOU ADD NEW Ciphers ^^^^^^^^^^^^^^^^^^
}

// DeriveKeyFromPassword takes a password string and applies the KDF to produce
// the derived key
func (c *Crypto) DeriveKeyFromPassword(password string) ([]byte, error) {
	switch c.KDF {
	// scrypt
	case DefaultKDF:
		return c.ScryptKeyFromPassword(password)
		// currently scrypt is the only supported KDF.
		// In the future add different KDF functions and a
		// case switch.
	}
	return nil, errors.New("unsupported key derivation function (scrypt only)")
	// CHANGE THIS IF YOU ADD NEW KDFs ^^^^^^^^^^^^^^^^^^
}

// WriteToFile writes an encrypted key to disk.
func (e *EncryptedKey) WriteToFile(filepath string) error {
	keyfileJSON, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath, keyfileJSON, 0644)
	return nil
}

// ReadFromEncryptedFile reads an encrypted key file (json format)
// and handles unmarshalling.
func ReadFromEncryptedFile(filepath string) (*EncryptedKey, error) {
	file, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	var ek EncryptedKey
	err = json.Unmarshal([]byte(file), &ek)
	if err != nil {
		return nil, err
	}
	return &ek, nil
}
