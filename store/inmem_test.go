package store

import (
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

func BenchmarkInmem(b *testing.B) {
	b.StopTimer()

	db := NewInmem()
	defer db.Close()

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
