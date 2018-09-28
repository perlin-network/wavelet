package wavelet

import (
	"testing"

	"encoding/binary"
	"github.com/stretchr/testify/assert"
)

func readAccountBalance(acct *Account) uint64 {
	val, ok := acct.Load("balance")
	if !ok {
		return 0
	} else {
		return binary.LittleEndian.Uint64(val)
	}
}

func TestNewAccount(t *testing.T) {
	t.Parallel()

	publicKey := "new_public_key"
	account := NewAccount(publicKey)

	assert.Equal(t, publicKey, account.PublicKey)
	assert.Equal(t, uint64(0), readAccountBalance(account))
	assert.Equal(t, uint64(0), account.Nonce)
}

func TestMarshal(t *testing.T) {
	t.Parallel()

	publicKey := "somekey"
	account := NewAccount(publicKey)

	b := account.MarshalBinary()
	assert.NotEqual(t, 0, len(b), "serialized account should not be length 0")
}
