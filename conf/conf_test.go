package conf

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	assert.EqualValues(t, 2, GetSnowballK())
	assert.EqualValues(t, 50, GetSnowballBeta())

	assert.EqualValues(t, 0.8, GetSyncVoteThreshold())
	assert.EqualValues(t, 0.8, GetFinalizationVoteThreshold())
	assert.EqualValues(t, 1, GetStakeMajorityWeight())
	assert.EqualValues(t, 0.3, GetTransactionsNumMajorityWeight())
	assert.EqualValues(t, 0.3, GetRoundDepthMajorityWeight())

	assert.EqualValues(t, 5000*time.Millisecond, GetQueryTimeout())
	assert.EqualValues(t, 5000*time.Millisecond, GetGossipTimeout())
	assert.EqualValues(t, 1*time.Second, GetDownloadTxTimeout())
	assert.EqualValues(t, 5000*time.Millisecond, GetCheckOutOfSyncTimeout())

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
		WithSnowballBeta(69),

		WithSyncVoteThreshold(1),
		WithFinalizationVoteThreshold(2),
		WithStakeMajorityWeight(3),
		WithTransactionsNumMajorityWeight(4),
		WithRoundDepthMajorityWeight(5),

		WithQueryTimeout(time.Second*10),
		WithGossipTimeout(time.Second*2),
		WithDownloadTxTimeout(time.Second*7),
		WithCheckOutOfSyncTimeout(time.Second*11),

		WithSyncChunkSize(666),
		WithSyncIfRoundsDifferBy(7),
		WithMaxDownloadDepthDiff(42),
		WithMaxDepthDiff(4),
		WithPruningLimit(13),
		WithSecret("shambles"),
	)

	assert.EqualValues(t, 10, GetSnowballK())
	assert.EqualValues(t, 69, GetSnowballBeta())

	assert.EqualValues(t, 1, GetSyncVoteThreshold())
	assert.EqualValues(t, 2, GetFinalizationVoteThreshold())
	assert.EqualValues(t, 3, GetStakeMajorityWeight())
	assert.EqualValues(t, 1.9, GetTransactionsNumMajorityWeight())
	assert.EqualValues(t, 1.9, GetRoundDepthMajorityWeight())

	assert.EqualValues(t, 10*time.Second, GetQueryTimeout())
	assert.EqualValues(t, 2*time.Second, GetGossipTimeout())
	assert.EqualValues(t, 7*time.Second, GetDownloadTxTimeout())
	assert.EqualValues(t, 11*time.Second, GetCheckOutOfSyncTimeout())

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
