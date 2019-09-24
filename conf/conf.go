package conf

import (
	"fmt"
	"sync"
	"time"
)

type config struct {
	// Snowball consensus protocol parameters.
	snowballK     int
	snowballAlpha float64
	snowballBeta  int

	// Timeout for outgoing requests
	queryTimeout  time.Duration
	gossipTimeout time.Duration

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

var (
	c = config{
		snowballK:            2,
		snowballAlpha:        0.8,
		snowballBeta:         50,
		queryTimeout:         500 * time.Millisecond,
		gossipTimeout:        500 * time.Millisecond,
		syncChunkSize:        16384,
		syncIfRoundsDifferBy: 2,
		maxDownloadDepthDiff: 1500,
		maxDepthDiff:         10,
		pruningLimit:         30,
	}

	l = sync.RWMutex{}
)

type Option func(*config)

func WithSnowballK(sk int) Option {
	return func(c *config) {
		c.snowballK = sk
	}
}

func WithSnowballAlpha(sa float64) Option {
	return func(c *config) {
		c.snowballAlpha = sa
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

func GetSnowballAlpha() float64 {
	l.RLock()
	t := c.snowballAlpha
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
	l.Unlock()
}

func Stringify() string {
	l.RLock()
	s := fmt.Sprintf("%+v", c)
	l.RUnlock()

	return s
}
