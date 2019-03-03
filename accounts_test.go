package wavelet

import (
	"bytes"
	"github.com/golang/snappy"
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

func TestSnappyDeterministic(t *testing.T) {
	fn := func(src [2 * 1024]byte) bool {
		return bytes.Equal(snappy.Encode(nil, src[:]), snappy.Encode(nil, src[:]))
	}

	assert.NoError(t, quick.Check(fn, &quick.Config{MaxCount: 10000}))
}
