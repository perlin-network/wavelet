package wavelet

import (
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestCriticalTimestamps(t *testing.T) {
	kv, err := store.NewRocksDB("db")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, kv.Delete(keyCriticalTimestamps[:]))
		assert.NoError(t, kv.Close())
	}()

	// ensure no saved timestamps returns empty slice
	tss, err := ReadCriticalTimestamps(kv)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(tss))

	// ensure one timestamp is successfully saved
	v := CriticalTimestampRecord{
		Timestamp: uint64(time.Now().UnixNano()),
		ViewID: 1,
	}
	if !assert.NoError(t, WriteCriticalTimestamp(kv, v.Timestamp, v.ViewID)) {
		return
	}

	tss, err = ReadCriticalTimestamps(kv)
	assert.NoError(t, err)
	if !assert.Equal(t, 1, len(tss)) {
		return
	}

	assert.Equal(t, v, tss[0])

	// ensure only predefined number of timestamps saved
	for i := 2; i < 7; i++ {
		if !assert.NoError(t, WriteCriticalTimestamp(kv, uint64(time.Now().UnixNano()), uint64(i))) {
			return
		}
	}

	tss, err = ReadCriticalTimestamps(kv)
	assert.NoError(t, err)
	if !assert.Equal(t, sys.CriticalTimestampAverageWindowSize, len(tss)) {
		return
	}

	// ensure timestamps for older views got evicted
	v = CriticalTimestampRecord{
		Timestamp: uint64(time.Now().UnixNano()),
		ViewID: 11,
	}
	if !assert.NoError(t, WriteCriticalTimestamp(kv, v.Timestamp, v.ViewID)) {
		return
	}

	tss, err = ReadCriticalTimestamps(kv)
	assert.NoError(t, err)
	if !assert.Equal(t, 1, len(tss)) {
		return
	}

	assert.Equal(t, v, tss[0])
}
