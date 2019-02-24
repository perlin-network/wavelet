package store

import (
	"crypto/rand"
	"github.com/stretchr/testify/assert"
	"testing"
)

func BenchmarkRocksDB(b *testing.B) {
	b.StopTimer()

	db, err := NewRocksDB("db")
	assert.NoError(b, err)
	defer db.Close()

	b.StartTimer()
	defer b.StopTimer()

	for i := 0; i < b.N; i++ {
		var randomKey [128]byte
		var randomValue [600]byte

		rand.Read(randomKey[:])
		rand.Read(randomValue[:])

		err = db.Put(randomKey[:], randomValue[:])
		if err != nil {
			b.Fatal(err)
		}

		value, err := db.Get(randomKey[:])
		if err != nil {
			b.Fatal(err)
		}

		assert.EqualValues(b, value, randomValue[:])
	}
}
