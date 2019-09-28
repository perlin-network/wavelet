package conf

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	assert.EqualValues(t, 2, GetSnowballK())
	assert.EqualValues(t, 0.8, GetSnowballAlpha())
	assert.EqualValues(t, 50, GetSnowballBeta())
	assert.EqualValues(t, 500*time.Millisecond, GetQueryTimeout())
	assert.EqualValues(t, 500*time.Millisecond, GetGossipTimeout())
	assert.EqualValues(t, 16384, GetSyncChunkSize())
	assert.EqualValues(t, 2, GetSyncIfRoundsDifferBy())
	assert.EqualValues(t, 1500, GetMaxDownloadDepthDiff())
	assert.EqualValues(t, 10, GetMaxDepthDiff())
	assert.EqualValues(t, 30, GetPruningLimit())
	assert.EqualValues(t, "", GetSecret())
}

func TestUpdate(t *testing.T) {
	defer resetConfig()

	Update(
		WithSnowballK(10),
		WithSnowballAlpha(0.123),
		WithSnowballBeta(69),
		WithQueryTimeout(time.Second*10),
		WithGossipTimeout(time.Second*2),
		WithSyncChunkSize(666),
		WithSyncIfRoundsDifferBy(7),
		WithMaxDownloadDepthDiff(42),
		WithMaxDepthDiff(4),
		WithPruningLimit(13),
		WithSecret("shambles"),
	)

	assert.EqualValues(t, 10, GetSnowballK())
	assert.EqualValues(t, 0.123, GetSnowballAlpha())
	assert.EqualValues(t, 69, GetSnowballBeta())
	assert.EqualValues(t, 10*time.Second, GetQueryTimeout())
	assert.EqualValues(t, 2*time.Second, GetGossipTimeout())
	assert.EqualValues(t, 666, GetSyncChunkSize())
	assert.EqualValues(t, 7, GetSyncIfRoundsDifferBy())
	assert.EqualValues(t, 42, GetMaxDownloadDepthDiff())
	assert.EqualValues(t, 4, GetMaxDepthDiff())
	assert.EqualValues(t, 13, GetPruningLimit())
	assert.EqualValues(t, "shambles", GetSecret())
}

func resetConfig() {
	c = defaultConfig()
}
