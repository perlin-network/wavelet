package wavelet

import (
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"sort"
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

	tss, err := ReadCriticalTimestamps(kv)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(tss))

	v := uint64(time.Now().UnixNano())
	if !assert.NoError(t, WriteCriticalTimestamp(kv, v)) {
		return
	}

	tss, err = ReadCriticalTimestamps(kv)
	assert.NoError(t, err)
	if !assert.Equal(t, 1, len(tss)) {
		return
	}

	assert.Equal(t, v, tss[0])

	for i := 0; i < 15; i++ {
		if !assert.NoError(t, WriteCriticalTimestamp(kv, uint64(time.Now().UnixNano()))) {
			return
		}
	}

	tss, err = ReadCriticalTimestamps(kv)
	assert.NoError(t, err)
	if !assert.Equal(t, CriticalTimestampsLimit, len(tss)) {
		return
	}

	assert.True(t, sort.SliceIsSorted(tss, func(i, j int) bool {return tss[i] <= tss[j]}))
}