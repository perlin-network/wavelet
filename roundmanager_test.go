package wavelet

import (
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRoundManager(t *testing.T) {
	//storage, err := store.NewLevelDB("./temp")
	//if !assert.NoError(t, err) {
	//	return
	//}
	//defer func() {
	//	if err := storage.Close(); err != nil {
	//		t.Fatal(err)
	//	}
	//}()
	storage := store.NewInmem()

	rm, err := NewRoundManager(10, storage)
	if !assert.EqualError(t, errors.Cause(err), "key not found") {
		return
	}

	if !assert.NotNil(t, rm) {
		return
	}

	for i := 0; i < 5; i++ {
		r := &Round{
			Index: uint64(i + 1),
		}
		_, err := rm.Save(r)
		assert.NoError(t, err)
	}

	assert.Equal(t, rm.Latest().Index, uint64(5))
	assert.Equal(t, rm.Oldest().Index, uint64(1))

	newRM, err := NewRoundManager(10, storage)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, rm.latest, newRM.latest)
	assert.Equal(t, rm.oldest, newRM.oldest)
	assert.Equal(t, len(rm.buff), len(newRM.buff))

	assert.Equal(t, newRM.Latest().Index, uint64(5))
	assert.Equal(t, newRM.Oldest().Index, uint64(1))
	assert.Equal(t, newRM.Count(), uint64(5))
}
