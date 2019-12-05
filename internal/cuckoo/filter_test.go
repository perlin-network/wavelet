package cuckoo

import (
	"bufio"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/blake2b"
	"os"
	"testing"
	"testing/quick"
)

func samples(t testing.TB) [][32]byte {
	f, _ := os.Open("/usr/share/dict/words")
	defer f.Close()

	scanner := bufio.NewScanner(f)

	var samples [][32]byte

	for scanner.Scan() {
		samples = append(samples, blake2b.Sum256([]byte(scanner.Text())))
	}

	return samples
}

/**
BenchmarkNewFilter-8               56608            221212 ns/op
BenchmarkInsert-8               328951662               36.5 ns/op
BenchmarkLookup-8               345647392               35.3 ns/op
BenchmarkMarshalBinary-8            8439           1330817 ns/op
BenchmarkUnmarshalBinary-8          2826           4143137 ns/o
*/

func BenchmarkNewFilter(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewFilter()
	}
}

func BenchmarkInsert(b *testing.B) {
	filter := NewFilter()

	samples := samples(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.Insert(samples[i%len(samples)])
	}
}

func BenchmarkLookup(b *testing.B) {
	filter := NewFilter()

	samples := samples(b)

	for _, sample := range samples {
		filter.Insert(sample)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.Lookup(samples[i%len(samples)])
	}
}

func BenchmarkMarshalBinary(b *testing.B) {
	filter := NewFilter()

	samples := samples(b)

	for _, sample := range samples {
		filter.Insert(sample)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.MarshalBinary()
	}
}

func BenchmarkUnmarshalBinary(b *testing.B) {
	filter := NewFilter()

	samples := samples(b)

	for _, sample := range samples {
		filter.Insert(sample)
	}

	data := filter.MarshalBinary()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := UnmarshalBinary(data)
		assert.NoError(b, err)
	}
}

func TestFilter(t *testing.T) {
	filter := NewFilter()

	samples := samples(t)

	if len(samples) > 1000 {
		samples = samples[:1000]
	}

	for _, sample := range samples {
		assert.True(t, filter.Insert(sample))
	}

	assert.EqualValues(t, filter.Count, len(samples))

	for _, sample := range samples {
		assert.False(t, filter.Insert(sample))
	}

	assert.EqualValues(t, filter.Count, len(samples))

	for _, sample := range samples {
		assert.True(t, filter.Lookup(sample))
	}

	for _, sample := range samples {
		assert.True(t, filter.Delete(sample))
	}

	assert.EqualValues(t, filter.Count, 0)

	for _, sample := range samples {
		assert.False(t, filter.Delete(sample))
	}

	for _, sample := range samples {
		assert.False(t, filter.Lookup(sample))
	}

	empty := NewFilter()
	filter.Reset()

	assert.Equal(t, filter, empty)
}

func TestEncoding(t *testing.T) {
	f := func(entries [][]byte) bool {
		a := NewFilter()

		for _, entry := range entries {
			if len(entry) > 100 {
				entry = entry[:100]
			}

			a.Insert(blake2b.Sum256(entry))
		}

		b, err := UnmarshalBinary(a.MarshalBinary())
		if !assert.NoError(t, err) {
			return false
		}

		if !assert.Equal(t, a, b) {
			return false
		}

		return true
	}

	assert.NoError(t, quick.Check(f, &quick.Config{MaxCount: 10}))
}
