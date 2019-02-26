package wavelet

import (
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

func TestSmartContract(t *testing.T) {
	db := store.NewInmem()
	defer func() {
		_ = db.Close()
	}()

	accounts := newAccounts(db)

	var id [TransactionIDSize]byte
	var code [2 * 1024 * 1024]byte

	_, err := rand.Read(id[:])
	assert.NoError(t, err)

	_, err = rand.Read(code[:])
	assert.NoError(t, err)

	// Not exist.
	returned, available := accounts.ReadAccountContractCode(id)
	assert.Nil(t, returned)
	assert.False(t, available)

	accounts.WriteAccountContractCode(id, code[:])

	returned, available = accounts.ReadAccountContractCode(id)
	assert.Equal(t, code[:], returned)
	assert.True(t, available)
}
