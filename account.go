package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet/iavl"
)

// Account represents a single account on Perlin.
type Account struct {
	State *iavl.Node

	Nonce uint64 `json:"nonce"`

	// PublicKey is the hex-encoded public key
	PublicKey []byte `json:"public_key"`
}

// MarshalBinary serializes this accounts IAVL+ tree.
func (a *Account) MarshalBinary() []byte {
	buffer := make([]byte, 8)
	binary.LittleEndian.PutUint64(buffer, a.Nonce)
	return append(buffer, a.State.Marshal()...)
}

func (a *Account) Range(callback func(k string, v []byte)) {
	if a.State != nil {
		a.State.Range(callback)
	}
}

func (a *Account) Load(key string) ([]byte, bool) {
	_, value := a.State.Load(key)
	return value, value != nil
}

func (a *Account) Store(key string, value []byte) {
	a.State, _ = a.State.Store(key, value)
}

func (a *Account) Clone() *Account {
	account := &Account{
		PublicKey: a.PublicKey,
		Nonce:     a.Nonce,
	}

	a.Range(func(k string, v []byte) {
		account.State, _ = account.State.Store(k, v)
	})

	return account
}

// Unmarshal decodes an accounts encoded bytes from the database.
func (a *Account) Unmarshal(encoded []byte) error {
	var err error

	a.Nonce = binary.LittleEndian.Uint64(encoded[:8])
	a.State, err = iavl.Unmarshal(encoded[8:])

	if err != nil {
		return err
	}

	return nil
}

// NewAccount returns a new account object with balance and nonce = 0
func NewAccount(publicKey []byte) *Account {
	account := &Account{
		PublicKey: publicKey,
	}

	return account
}
