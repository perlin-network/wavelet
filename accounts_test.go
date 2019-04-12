package wavelet

import (
	"bytes"
	"github.com/golang/snappy"
	"github.com/perlin-network/wavelet/common"
	"github.com/stretchr/testify/assert"
	"testing"
	"testing/quick"
)

func TestSmartContract(t *testing.T) {
	fn := func(id common.TransactionID, code [2 * 1024]byte) bool {
		db, cleanup := GetKV("level", "db")
		defer cleanup()

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

func TestSnappyDeterministic(t *testing.T) {
	fn := func(src [2 * 1024]byte) bool {
		return bytes.Equal(snappy.Encode(nil, src[:]), snappy.Encode(nil, src[:]))
	}

	assert.NoError(t, quick.Check(fn, &quick.Config{MaxCount: 1000}))
}
