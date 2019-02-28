package wavelet

import (
	"bytes"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"testing"
	"testing/quick"
)

func TestSmartContract(t *testing.T) {
	fn := func(id [TransactionIDSize]byte, code [2 * 1024]byte) bool {
		db := store.NewInmem()
		defer func() {
			_ = db.Close()
		}()

		accounts := newAccounts(db)

		returned, available := accounts.ReadAccountContractCode(id)
		if returned != nil || available == true {
			return false
		}

		accounts.WriteAccountContractCode(id, code[:])

		returned, available = accounts.ReadAccountContractCode(id)
		if !bytes.Equal(code[:], returned) || available == false {
			return false
		}

		return true
	}

	assert.NoError(t, quick.Check(fn, nil))

}
