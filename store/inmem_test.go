package store

import (
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

func BenchmarkInmem(b *testing.B) {
	b.StopTimer()

	db := NewInmem()
	defer func() {
		_ = db.Close()
	}()

	b.StartTimer()
	defer b.StopTimer()

	for i := 0; i < b.N; i++ {
		var randomKey [128]byte
		var randomValue [600]byte

		_, err := rand.Read(randomKey[:])
		assert.NoError(b, err)
		_, err = rand.Read(randomValue[:])
		assert.NoError(b, err)

		err = db.Put(randomKey[:], randomValue[:])
		assert.NoError(b, err)

		value, err := db.Get(randomKey[:])
		assert.NoError(b, err)

		assert.EqualValues(b, randomValue[:], value)
	}
}

func TestExistence(t *testing.T) {
	db := NewInmem()
	defer func() {
		_ = db.Close()
	}()

	_, err := db.Get([]byte("not_exist"))
	assert.Error(t, err)

	err = db.Put([]byte("exist"), []byte{})
	assert.NoError(t, err)

	val, err := db.Get([]byte("exist"))
	assert.NoError(t, err)
	assert.Equal(t, []byte{}, val)
}
