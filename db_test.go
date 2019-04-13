package wavelet

import (
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestCriticalTimestamps(t *testing.T) {
	t.Run("empty database returns nil array of timestamps", func(t *testing.T) {
		kv := store.NewInmem()
		assert.Empty(t, ReadCriticalTimestamps(kv))
	})

	t.Run("can save one timestamp", func(t *testing.T) {
		kv := store.NewInmem()

		v, ts := uint64(1), uint64(time.Now().UnixNano())

		assert.NoError(t, WriteCriticalTimestamp(kv, v, ts))

		timestamps := ReadCriticalTimestamps(kv)
		assert.Len(t, timestamps, 1)

		assert.Equal(t, ts, timestamps[v])
	})

	t.Run("can save multiple timestamps", func(t *testing.T) {
		kv := store.NewInmem()

		for v := uint64(1); v <= uint64(sys.CriticalTimestampAverageWindowSize); v++ {
			assert.NoError(t, WriteCriticalTimestamp(kv, v, uint64(time.Now().UnixNano())))
		}

		timestamps := ReadCriticalTimestamps(kv)
		assert.Len(t, timestamps, sys.CriticalTimestampAverageWindowSize)
	})

	t.Run("can evict multiple timestamps", func(t *testing.T) {
		kv := store.NewInmem()

		for v := uint64(1); v <= uint64(sys.CriticalTimestampAverageWindowSize); v++ {
			assert.NoError(t, WriteCriticalTimestamp(kv, v, uint64(time.Now().UnixNano())))
		}

		assert.NoError(t, WriteCriticalTimestamp(kv, uint64(sys.CriticalTimestampAverageWindowSize+2), uint64(time.Now().UnixNano())))

		timestamps := ReadCriticalTimestamps(kv)
		assert.Len(t, timestamps, sys.CriticalTimestampAverageWindowSize)

		assert.NotZero(t, timestamps[uint64(sys.CriticalTimestampAverageWindowSize)+2])
		assert.NotZero(t, timestamps[uint64(sys.CriticalTimestampAverageWindowSize)])
		assert.NotZero(t, timestamps[uint64(sys.CriticalTimestampAverageWindowSize)-1])
		assert.Zero(t, timestamps[1])

		assert.NoError(t, WriteCriticalTimestamp(kv, uint64(sys.CriticalTimestampAverageWindowSize*4), uint64(time.Now().UnixNano())))

		timestamps = ReadCriticalTimestamps(kv)
		assert.Len(t, timestamps, 1)
	})
}
