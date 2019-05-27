package wavelet

import (
	"bytes"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"testing"
	"testing/quick"
)

func TestSmartContract(t *testing.T) {
	fn := func(id TransactionID, code [2 * 1024]byte) bool {
		accounts := NewAccounts(store.NewInmem())
		tree := accounts.Snapshot()

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
