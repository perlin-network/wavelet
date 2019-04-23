package wavelet

import (
	"bytes"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"testing"
	"testing/quick"
)

func TestSmartContract(t *testing.T) {
	fn := func(id common.TransactionID, code [2 * 1024]byte) bool {
		db := store.NewInmem()
		defer func() {
			_ = db.Close()
		}()

		accounts := newAccounts(db)
		tree := accounts.snapshot()

		returned, available := ReadAccountContractCode(tree, id)
		if returned != nil || available == true {
			return false
		}

		WriteAccountContractCode(tree, id, code[:])

		returned, available = ReadAccountContractCode(tree, id)
		if !bytes.Equal(code[:], returned) || available == false {
			return false
		}

		return true
	}

	assert.NoError(t, quick.Check(fn, nil))
}
