package wavelet

import (
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewRounds(t *testing.T) {
	storage := store.NewInmem()

	rm, err := NewRounds(storage, 10)
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

	assert.Equal(t, uint64(5), rm.Latest().Index)
	assert.Equal(t, uint64(1), rm.Oldest().Index)

	newRM, err := NewRounds(storage, 10)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, rm.latest, newRM.latest)
	assert.Equal(t, rm.oldest, newRM.oldest)
	assert.Equal(t, len(rm.buffer), len(newRM.buffer))

	assert.Equal(t, uint64(5), newRM.Latest().Index)
	assert.Equal(t, uint64(1), newRM.Oldest().Index)
	assert.Equal(t, uint64(5), newRM.Count())
}

func TestRoundsCircular(t *testing.T) {
	storage := store.NewInmem()

	rm, err := NewRounds(storage, 10)
	if !assert.EqualError(t, errors.Cause(err), "key not found") {
		return
	}

	if !assert.NotNil(t, rm) {
		return
	}

	for i := 0; i < 15; i++ {
		r := &Round{
			Index: uint64(i + 1),
		}
		_, err := rm.Save(r)
		assert.NoError(t, err)
	}

	assert.Equal(t, uint64(15), rm.Latest().Index)
	assert.Equal(t, uint64(6), rm.Oldest().Index)

	newRM, err := NewRounds(storage, 10)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, rm.latest, newRM.latest)
	assert.Equal(t, rm.oldest, newRM.oldest)
	assert.Equal(t, len(rm.buffer), len(newRM.buffer))

	assert.Equal(t, uint32(4), newRM.latest)
	assert.Equal(t, uint32(5), newRM.oldest)
}