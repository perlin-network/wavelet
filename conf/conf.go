package conf

import (
	"fmt"
	"sync"
	"time"
)

type config struct {
	// Snowball consensus protocol parameters.
	snowballK    int
	snowballBeta int

	// votes counting and majority calculation related parameters
	syncVoteThreshold             float64
	finalizationVoteThreshold     float64
	stakeMajorityWeight           float64
	transactionsNumMajorityWeight float64
	roundDepthMajorityWeight      float64

	// Timeout for outgoing requests
	queryTimeout          time.Duration
	gossipTimeout         time.Duration
	downloadTxTimeout     time.Duration
	checkOutOfSyncTimeout time.Duration

	// Size of individual chunks sent for a syncing peer.
	syncChunkSize int

	// Number of rounds we should be behind before we start syncing.
	syncIfRoundsDifferBy uint64

	// Depth diff according to which transactions are downloaded to sync
	maxDownloadDepthDiff uint64

	// Max graph depth difference to search for eligible transaction
	// parents from for our node.
	maxDepthDiff uint64

	// Number of rounds after which transactions will be pruned from the graph
	pruningLimit uint8

	// shared secret for http api authorization
	secret string
}

var c config
var l sync.RWMutex

func init() {
	c = defaultConfig()
	l = sync.RWMutex{}
}

func defaultConfig() config {
	return config{
		snowballK:    2,
		snowballBeta: 50,

		syncVoteThreshold:             0.8,
		finalizationVoteThreshold:     0.8,
		stakeMajorityWeight:           1,
		transactionsNumMajorityWeight: 0.3,
		roundDepthMajorityWeight:      0.3,

		queryTimeout:          5000 * time.Millisecond,
		gossipTimeout:         5000 * time.Millisecond,
		downloadTxTimeout:     1 * time.Second,
		checkOutOfSyncTimeout: 5000 * time.Millisecond,
		syncChunkSize:         16384,
		syncIfRoundsDifferBy:  2,

		maxDownloadDepthDiff: 1500,
		maxDepthDiff:         10,
		pruningLimit:         30,
	}
}

type Option func(*config)

// updateWeights update transaction number and round depth weights for consensus vote counting
// weights values are calculated as such, that it should be just enough to reach majority
// in case of same stakes
func updateWeights() {
	c.transactionsNumMajorityWeight = c.finalizationVoteThreshold - (1 / float64(c.snowballK))
	c.roundDepthMajorityWeight = c.finalizationVoteThreshold - (1 / float64(c.snowballK))
}

func WithSnowballK(sk int) Option {
	return func(c *config) {
		c.snowballK = sk
	}
}

func WithSyncVoteThreshold(sa float64) Option {
	return func(c *config) {
		c.syncVoteThreshold = sa
	}
}

func WithFinalizationVoteThreshold(sa float64) Option {
	return func(c *config) {
		c.finalizationVoteThreshold = sa
	}
}

func WithStakeMajorityWeight(sa float64) Option {
	return func(c *config) {
		c.stakeMajorityWeight = sa
	}
}

func WithTransactionsNumMajorityWeight(sa float64) Option {
	return func(c *config) {
		c.transactionsNumMajorityWeight = sa
	}
}

func WithRoundDepthMajorityWeight(sa float64) Option {
	return func(c *config) {
		c.roundDepthMajorityWeight = sa
	}
}

func WithSnowballBeta(sb int) Option {
	return func(c *config) {
		c.snowballBeta = sb
	}
}

func WithQueryTimeout(qt time.Duration) Option {
	return func(c *config) {
		c.queryTimeout = qt
	}
}

func WithGossipTimeout(gt time.Duration) Option {
	return func(c *config) {
		c.gossipTimeout = gt
	}
}

func WithDownloadTxTimeout(dt time.Duration) Option {
	return func(c *config) {
		c.downloadTxTimeout = dt
	}
}

func WithCheckOutOfSyncTimeout(ct time.Duration) Option {
	return func(c *config) {
		c.checkOutOfSyncTimeout = ct
	}
}

func WithSyncChunkSize(cs int) Option {
	return func(c *config) {
		c.syncChunkSize = cs
	}
}

func WithSyncIfRoundsDifferBy(rdb uint64) Option {
	return func(c *config) {
		c.syncIfRoundsDifferBy = rdb
	}
}

func WithMaxDownloadDepthDiff(ddd uint64) Option {
	return func(c *config) {
		c.maxDownloadDepthDiff = ddd
	}
}

func WithMaxDepthDiff(dd uint64) Option {
	return func(c *config) {
		c.maxDepthDiff = dd
	}
}

func WithPruningLimit(pl uint8) Option {
	return func(c *config) {
		c.pruningLimit = pl
	}
}

func WithSecret(s string) Option {
	return func(c *config) {
		c.secret = s
	}
}

func GetSnowballK() int {
	l.RLock()
	t := c.snowballK
	l.RUnlock()

	return t
}

func GetSyncVoteThreshold() float64 {
	l.RLock()
	t := c.syncVoteThreshold
	l.RUnlock()

	return t
}

func GetFinalizationVoteThreshold() float64 {
	l.RLock()
	t := c.finalizationVoteThreshold
	l.RUnlock()

	return t
}

func GetStakeMajorityWeight() float64 {
	l.RLock()
	t := c.stakeMajorityWeight
	l.RUnlock()

	return t
}

func GetTransactionsNumMajorityWeight() float64 {
	l.RLock()
	t := c.transactionsNumMajorityWeight
	l.RUnlock()

	return t
}

func GetRoundDepthMajorityWeight() float64 {
	l.RLock()
	t := c.roundDepthMajorityWeight
	l.RUnlock()

	return t
}

func GetSnowballBeta() int {
	l.RLock()
	t := c.snowballBeta
	l.RUnlock()

	return t
}

func GetQueryTimeout() time.Duration {
	l.RLock()
	t := c.queryTimeout
	l.RUnlock()

	return t
}

func GetGossipTimeout() time.Duration {
	l.RLock()
	t := c.gossipTimeout
	l.RUnlock()

	return t
}

func GetDownloadTxTimeout() time.Duration {
	l.RLock()
	t := c.downloadTxTimeout
	l.RUnlock()

	return t
}

func GetCheckOutOfSyncTimeout() time.Duration {
	l.RLock()
	t := c.checkOutOfSyncTimeout
	l.RUnlock()

	return t
}

func GetSyncChunkSize() int {
	l.RLock()
	t := c.syncChunkSize
	l.RUnlock()

	return t
}

func GetSyncIfRoundsDifferBy() uint64 {
	l.RLock()
	t := c.syncIfRoundsDifferBy
	l.RUnlock()

	return t
}

func GetMaxDownloadDepthDiff() uint64 {
	l.RLock()
	t := c.maxDownloadDepthDiff
	l.RUnlock()

	return t
}

func GetMaxDepthDiff() uint64 {
	l.RLock()
	t := c.maxDepthDiff
	l.RUnlock()

	return t
}

func GetPruningLimit() uint8 {
	l.RLock()
	t := c.pruningLimit
	l.RUnlock()

	return t
}

func GetSecret() string {
	l.RLock()
	t := c.secret
	l.RUnlock()

	return t
}

func Update(options ...Option) {
	l.Lock()

	for _, option := range options {
		option(&c)
	}

	updateWeights()

	l.Unlock()
}

func Stringify() string {
	l.RLock()
	s := fmt.Sprintf("%+v", c)
	l.RUnlock()

	return s
}
