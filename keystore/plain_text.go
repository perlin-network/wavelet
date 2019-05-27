package keystore

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"time"

	"github.com/perlin-network/noise/edwards25519"
)

// PlainTextKey is the structure for plain text keys.
type PlainTextKey struct {
	Account     string    `json:"account"`
	PrivateKey  string    `json:"key"`
	Version     int       `json:"version"`
	TimeCreated time.Time `json:"created"`
}

// NewPlainTextKey generates a new plain text key struct.
func NewPlainTextKey(privateKey edwards25519.PrivateKey) *PlainTextKey {
	publicKey := privateKey.Public()
	publicKeyHex := hex.EncodeToString(publicKey[:])
	privateKeyHex := hex.EncodeToString(privateKey[:])
	ptk := &PlainTextKey{
		Account:     publicKeyHex,
		Version:     Version,
		TimeCreated: time.Now(),
		PrivateKey:  privateKeyHex,
	}
	return ptk
}

// ExtractFromPlainTextKey takes a plain text struct and pulls out the private key.
func (pt *PlainTextKey) ExtractFromPlainTextKey() (*edwards25519.PrivateKey, error) {
	if len(pt.PrivateKey) == hex.EncodedLen(edwards25519.SizePrivateKey) {
		privateKeyBytes, err := hex.DecodeString(pt.PrivateKey)
		if err != nil {
			return nil, err
		}
		var privateKey edwards25519.PrivateKey
		copy(privateKey[:], privateKeyBytes[:edwards25519.SizePrivateKey])
		return &privateKey, nil
	}
	return nil, errors.New("did not work")
}

// WriteToFile writes a plain text struct to disk (json).
func (pt *PlainTextKey) WriteToFile(filepath string) error {
	keyfileJSON, err := json.MarshalIndent(pt, "", "  ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath, keyfileJSON, 0644)
	return nil
}

// ReadFromPlainTextFile reads a plain text key file and handles unmarshalling.
func ReadFromPlainTextFile(filepath string) (*PlainTextKey, error) {
	file, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	var pt PlainTextKey
	err = json.Unmarshal([]byte(file), &pt)
	if err != nil {
		return nil, err
	}

	return &pt, nil
}
