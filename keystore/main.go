package keystore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"errors"
	"io"
	"time"

	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/scrypt"

	"github.com/mitchellh/mapstructure"
	"github.com/perlin-network/noise/edwards25519"
)

// Version ..
const (
	Version int = 1

	DefaultKDF    = "scrypt"
	DefaultCipher = "secretbox"

	DefaultScryptN      int = 262144
	DefaultScryptP      int = 1
	DefaultScryptR      int = 8
	DefaultScryptKeyLen int = 32
	DefaultSaltLen      int = 32
)

// PlainTextKey ...
type PlainTextKey struct {
	Account     string    `json:"account"`
	PrivateKey  string    `json:"key"`
	Version     int       `json:"version"`
	TimeCreated time.Time `json:"created"`
}

// EncryptedKey contains details about the Cipher / KDF parameters,
// the encrypted key (cipher text), and account details for easy
// tracking.
type EncryptedKey struct {
	Account     string    `json:"account"`
	Version     int       `json:"version"`
	TimeCreated time.Time `json:"created"`
	Crypto      Crypto    `json:"crypto"`
}

// Crypto contains all of the parameters for the cipher and KDF.
type Crypto struct {
	Cipher       string      `json:"cipher"`
	CipherText   string      `json:"cipherText"`
	CipherParams interface{} `json:"cipherParams"`
	KDF          string      `json:"kdf"`
	KDFParams    interface{} `json:"kdfParams"`
}

// CryptoOptions ...
type CryptoOptions struct {
	Cipher       string
	KDF          string
	CipherParams interface{}
	KDFParams    interface{}
}

// ScryptParams ...
type ScryptParams struct {
	N      int    `json:"n"`
	P      int    `json:"p"`
	R      int    `json:"r"`
	KeyLen int    `json:"keyLen"`
	Salt   string `json:"salt"`
}

// SecretboxParams ...
type SecretboxParams struct {
	Nonce string `json:"nonce"`
}

// NewEncryptedKey ...
func NewEncryptedKey(privateKey edwards25519.PrivateKey, password string) (EncryptedKey, error) {
	publicKey := privateKey.Public()
	accountID := hex.EncodeToString(publicKey[:])
	ek := EncryptedKey{
		Account:     accountID,
		Version:     Version,
		TimeCreated: time.Now(),
		Crypto:      Crypto{},
	}
	err := ek.Crypto.SetDefaultCipher()
	if err != nil {
		return EncryptedKey{}, err
	}
	err = ek.Crypto.SetDefaultKDFParams()
	if err != nil {
		return EncryptedKey{}, err
	}

	dk, err := ek.Crypto.DeriveKeyFromPassword(password)
	if err != nil {
		return EncryptedKey{}, err
	}

	err = ek.Crypto.EncryptPrivateKey(privateKey, dk)
	if err != nil {
		return EncryptedKey{}, err
	}
	return ek, nil
}

// NewEncryptedKeyWithOptions ...
func NewEncryptedKeyWithOptions(privateKey edwards25519.PrivateKey, password string, opts CryptoOptions) (EncryptedKey, error) {
	publicKey := privateKey.Public()
	accountID := hex.EncodeToString(publicKey[:])
	ek := EncryptedKey{
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
		return EncryptedKey{}, err
	}

	err = ek.Crypto.EncryptPrivateKey(privateKey, dk)
	if err != nil {
		return EncryptedKey{}, err
	}
	return ek, nil
}

// Decrypt ..
func (e *EncryptedKey) Decrypt(password string) (edwards25519.PrivateKey, error) {
	dk, err := e.Crypto.DeriveKeyFromPassword(password)
	if err != nil {
		return edwards25519.PrivateKey{}, err
	}
	privKey, err := e.Crypto.DecryptPrivateKey(dk)
	if err != nil {
		return edwards25519.PrivateKey{}, err
	}
	return privKey, nil
}

// EncryptPrivateKey ...
func (c *Crypto) EncryptPrivateKey(privateKey edwards25519.PrivateKey, derivedKey []byte) error {
	switch c.Cipher {
	// secretbox
	case DefaultCipher:
		return c.SecretBoxEncrypt(privateKey, derivedKey)
	}
	return errors.New("unsupported cipher (secretbox only)")
	// CHANGE THIS IF YOU ADD NEW Ciphers ^^^^^^^^^^^^^^^^^^
}

// DecryptPrivateKey ...
func (c *Crypto) DecryptPrivateKey(derivedKey []byte) (edwards25519.PrivateKey, error) {
	switch c.Cipher {
	// secretbox
	case DefaultCipher:
		return c.SecretBoxDecrypt(derivedKey)
	}
	return edwards25519.PrivateKey{}, errors.New("unsupported cipher (secretbox only)")
	// CHANGE THIS IF YOU ADD NEW Ciphers ^^^^^^^^^^^^^^^^^^
}

// SecretBoxEncrypt ..
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

// SecretBoxDecrypt ..
func (c *Crypto) SecretBoxDecrypt(derivedKey []byte) (edwards25519.PrivateKey, error) {
	if c.CipherParams == nil {
		return edwards25519.PrivateKey{}, errors.New("secretbox cipher parameters nil")
		// fail early
	} else if len(c.CipherText) == 0 {
		return edwards25519.PrivateKey{}, errors.New("secretbox cipher text is not set")
	}
	var params SecretboxParams
	err := mapstructure.Decode(c.CipherParams, &params)
	if err != nil {
		return edwards25519.PrivateKey{}, err
	}
	c.CipherParams = params
	nonce, err := hex.DecodeString(params.Nonce)
	if err != nil {
		return edwards25519.PrivateKey{}, err
	}
	cipherText, err := hex.DecodeString(c.CipherText)
	if err != nil {
		return edwards25519.PrivateKey{}, err
	}

	if len(nonce) == 24 && len(derivedKey) == 32 {
		var nonceArr [24]byte
		copy(nonceArr[:], nonce)
		var derivedKeyArr [32]byte
		copy(derivedKeyArr[:], derivedKey)
		privateKey, ok := secretbox.Open(nil, cipherText, &nonceArr, &derivedKeyArr)
		if !ok {
			return edwards25519.PrivateKey{}, errors.New("could not open secret box cipher text")
		}
		var privateKeyArr [edwards25519.SizePrivateKey]byte
		copy(privateKeyArr[:], privateKey)

		return privateKeyArr, nil
	}
	return edwards25519.PrivateKey{}, errors.New("nonce or derived key were incorrect lengths")
}

// DeriveKeyFromPassword ...
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

// ScryptKeyFromPassword ..
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

// SetDefaultKDFParams ...
func (c *Crypto) SetDefaultKDFParams() error {
	c.KDF = DefaultKDF
	salt, err := NewRandomSalt(DefaultSaltLen)
	if err != nil {
		return err
	}
	c.KDFParams = ScryptParams{
		N:      DefaultScryptN,
		R:      DefaultScryptR,
		P:      DefaultScryptP,
		KeyLen: DefaultScryptKeyLen,
		Salt:   hex.EncodeToString(salt),
	}
	return nil
}

// SetDefaultCipher ..
func (c *Crypto) SetDefaultCipher() error {
	c.Cipher = DefaultCipher
	nonce, err := NewRandomNonce()
	if err != nil {
		return err
	}
	c.CipherParams = SecretboxParams{Nonce: hex.EncodeToString(nonce[:])}
	return nil
}

// NewRandomSalt ...
func NewRandomSalt(len int) ([]byte, error) {
	salt := make([]byte, len)
	_, err := io.ReadFull(rand.Reader, salt)
	if err != nil {
		return nil, err
	}
	return salt, nil
}

// NewRandomNonce ...
func NewRandomNonce() ([24]byte, error) {
	var nonce [24]byte
	_, err := io.ReadFull(rand.Reader, nonce[:])
	if err != nil {
		return [24]byte{}, err
	}
	return nonce, nil
}

// WriteToFile ...
func (e *EncryptedKey) WriteToFile() error {
	keyfileJSON, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile("key.json", keyfileJSON, 0644)
	return nil
}

// ReadFromFile ...
func ReadFromFile() (EncryptedKey, error) {
	file, err := ioutil.ReadFile("key.json")
	if err != nil {
		return EncryptedKey{}, err
	}
	var ek EncryptedKey
	err = json.Unmarshal([]byte(file), &ek)
	if err != nil {
		return EncryptedKey{}, err
	}
	fmt.Println(ek)

	return ek, nil
}
