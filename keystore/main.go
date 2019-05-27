package keystore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"

	"io"
	"os"
)

const (
	// Version identifies what keystore version was used on the file.
	Version int = 1

	// PlainTextFileSizeUpperBound is the byte size upper bound for plain text key files.
	PlainTextFileSizeUpperBound = 300

	// DefaultKDF is the default KDF (scrypt)
	DefaultKDF = "scrypt"
	// DefaultCipher is the default cipher (secretbox)
	DefaultCipher = "secretbox"

	// DefaultScryptN = 262144
	DefaultScryptN int = 262144
	// DefaultScryptP = 1
	DefaultScryptP int = 1
	// DefaultScryptR = 8
	DefaultScryptR int = 8
	// DefaultScryptKeyLen = 32
	DefaultScryptKeyLen int = 32
	// DefaultSaltLen = 32
	DefaultSaltLen int = 32
)

// Crypto contains all of the parameters for the cipher and KDF.
type Crypto struct {
	Cipher       string      `json:"cipher"`
	CipherText   string      `json:"cipherText"`
	CipherParams interface{} `json:"cipherParams"`
	KDF          string      `json:"kdf"`
	KDFParams    interface{} `json:"kdfParams"`
}

// CryptoOptions allows the crypto parameters to be manually set.
type CryptoOptions struct {
	Cipher       string
	KDF          string
	CipherParams interface{}
	KDFParams    interface{}
}

// SetDefaultKDFParams sets the KDF params to the defaults.
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

// SetDefaultCipher sets the cipher params to the defaults.
func (c *Crypto) SetDefaultCipher() error {
	c.Cipher = DefaultCipher
	nonce, err := NewRandomNonce()
	if err != nil {
		return err
	}
	c.CipherParams = SecretboxParams{Nonce: hex.EncodeToString(nonce[:])}
	return nil
}

// NewRandomSalt generates a random salt from the rand.Reader.
func NewRandomSalt(len int) ([]byte, error) {
	salt := make([]byte, len)
	_, err := io.ReadFull(rand.Reader, salt)
	if err != nil {
		return nil, err
	}
	return salt, nil
}

// NewRandomNonce generates a nonce from rand.Reader.
func NewRandomNonce() ([24]byte, error) {
	var nonce [24]byte
	_, err := io.ReadFull(rand.Reader, nonce[:])
	if err != nil {
		return [24]byte{}, err
	}
	return nonce, nil
}

// Write is a generic for writing key structs to disk. Handles
// encrypted and plain text keys differently.
func Write(filepath string, keyStruct interface{}) error {
	switch keyStruct.(type) {
	case *PlainTextKey:
		err := keyStruct.(*PlainTextKey).WriteToFile(filepath)
		if err != nil {
			return err
		}
	case *EncryptedKey:
		err := keyStruct.(*EncryptedKey).WriteToFile(filepath)
		if err != nil {
			return err
		}
	default:
		return errors.New("unsupported key struct")
	}
	return nil
}

// IsProbablyEncrypted checks the size of a file and uses that to make
// and educated guess whether the file is encrypted.
func IsProbablyEncrypted(filepath string) (bool, error) {
	fi, err := os.Stat(filepath)
	if err != nil {
		return false, err
	}
	if fi.Size() > PlainTextFileSizeUpperBound {
		return true, nil
	}
	return false, nil

}
