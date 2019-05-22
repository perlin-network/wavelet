package wavelet

import (
	"fmt"
	"github.com/perlin-network/wavelet/store"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
	"time"
)

func TestRoundManager(t *testing.T) {
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

	assert.Equal(t, uint64(5), rm.Latest().Index)
	assert.Equal(t, uint64(1), rm.Oldest().Index)

	newRM, err := NewRoundManager(10, storage)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, rm.latest, newRM.latest)
	assert.Equal(t, rm.oldest, newRM.oldest)
	assert.Equal(t, len(rm.buff), len(newRM.buff))

	assert.Equal(t, uint64(5), newRM.Latest().Index)
	assert.Equal(t, uint64(1), newRM.Oldest().Index)
	assert.Equal(t, uint64(5), newRM.Count())
}

func TestRoundManager_Ring(t *testing.T) {
	storage := store.NewInmem()

	rm, _ := NewRoundManager(10, storage)

	for i := 0; i < 15; i++ {
		r := &Round{
			Index: uint64(i + 1),
		}
		_, err := rm.Save(r)
		assert.NoError(t, err)
	}

	assert.Equal(t, uint64(15), rm.Latest().Index)
	assert.Equal(t, uint64(6), rm.Oldest().Index)

	newRM, err := NewRoundManager(10, storage)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, rm.latest, newRM.latest)
	assert.Equal(t, rm.oldest, newRM.oldest)
	assert.Equal(t, len(rm.buff), len(newRM.buff))

	assert.Equal(t, uint32(4), newRM.latest)
	assert.Equal(t, uint32(5), newRM.oldest)
}

func TestRoundManager_OverwrittenRound(t *testing.T) {
	storage := store.NewInmem()

	rm, _ := NewRoundManager(3, storage)

	for i := 0; i < 3; i++ {
		r := &Round{
			Index: uint64(i + 1),
		}
		_, err := rm.Save(r)
		assert.NoError(t, err)
	}

	expected := rm.Oldest()
	round, err := rm.Save(new(Round))
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, expected, round)
}

func TestRoundManager_GetRound(t *testing.T) {
	storage := store.NewInmem()

	rm, _ := NewRoundManager(3, storage)

	for i := 0; i < 3; i++ {
		r := &Round{
			Index: uint64(i + 1),
		}
		_, err := rm.Save(r)
		assert.NoError(t, err)
	}

	round, err := rm.GetRound(2)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, uint64(2), round.Index)
}

func BenchmarkRoundManager_GetRound(b *testing.B) {
	rand.Seed(time.Now().Unix())

	limit := 255
	storage := store.NewInmem()
	rm, _ := NewRoundManager(uint8(limit), storage)

	var err error
	for i := 0; i < limit; i++ {
		_, err = rm.Save(&Round{Index: uint64(i)})
	}

	for i := 0; i < b.N; i++ {
		_, err = rm.GetRound(uint64(rand.Intn(limit)))
	}

	fmt.Println(err)
}
