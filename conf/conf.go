package conf

import (
	"fmt"
	"sync"
	"time"

	"github.com/perlin-network/wavelet/sys"
)

type config struct {
	// Snowball consensus protocol parameters.
	snowballK     int
	snowballBeta  int
	SnowballAlpha float64

	// Timeout for outgoing requests
	queryTimeout          time.Duration
	gossipTimeout         time.Duration
	downloadTxTimeout     time.Duration
	checkOutOfSyncTimeout time.Duration

	// transaction syncing parameters
	txSyncChunkSize uint64
	txSyncLimit     uint64

	// max number of missing transactions to be pulled at once
	missingTxPullLimit uint64

	// Size of individual chunks sent for a syncing peer.
	syncChunkSize int

	// Number of blocks we should be behind before we start syncing.
	syncIfBlockIndicesDifferBy uint64

	// Number of blocks after which transactions will be pruned from the graph
	pruningLimit uint8

	// Max number of transactions within the block
	blockTxLimit uint64

	// shared secret for http api authorization
	secret string
}

var (
	l sync.RWMutex

	defaultConf = defaultConfig()
	c           = defaultConf
)

func defaultConfig() config {
	defConf := config{
		snowballK:     2,
		snowballBeta:  150,
		SnowballAlpha: 0.8,

		queryTimeout:               5 * time.Second,
		gossipTimeout:              5 * time.Second,
		downloadTxTimeout:          30 * time.Second,
		checkOutOfSyncTimeout:      5 * time.Second,
		syncChunkSize:              16384,
		syncIfBlockIndicesDifferBy: 5,

		txSyncChunkSize: 5000,
		txSyncLimit:     1 << 20,

		missingTxPullLimit: 5000,

		pruningLimit: 30,

		blockTxLimit: 1 << 16,
	}

	if sys.VersionMeta == "testnet" {
		defConf.snowballK = 10
	}

	return defConf
}

type Option func(*config)

func WithSnowballK(sk int) Option {
	return func(c *config) {
		c.snowballK = sk
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

func WithSyncIfBlockIndicesDifferBy(rdb uint64) Option {
	return func(c *config) {
		c.syncIfBlockIndicesDifferBy = rdb
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

func WithTXSyncChunkSize(n uint64) Option {
	return func(c *config) {
		c.txSyncChunkSize = n
	}
}

func WithTXSyncLimit(n uint64) Option {
	return func(c *config) {
		c.txSyncLimit = n
	}
}

func WithBlockTXLimit(n uint64) Option {
	return func(c *config) {
		c.blockTxLimit = n
	}
}

func WithMissingTxPullLimit(n uint64) Option {
	return func(c *config) {
		c.missingTxPullLimit = n
	}
}

func GetSnowballK() int {
	l.RLock()
	t := c.snowballK
	l.RUnlock()

	return t
}

func GetSnowballBeta() int {
	l.RLock()
	t := c.snowballBeta
	l.RUnlock()

	return t
}

func GetSnowballAlpha() float64 {
	l.RLock()
	t := c.SnowballAlpha
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

func GetSyncIfBlockIndicesDifferBy() uint64 {
	l.RLock()
	t := c.syncIfBlockIndicesDifferBy
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

func GetTXSyncChunkSize() uint64 {
	l.RLock()
	t := c.txSyncChunkSize
	l.RUnlock()

	return t
}

func GetTXSyncLimit() uint64 {
	l.RLock()
	t := c.txSyncLimit
	l.RUnlock()

	return t
}

func GetBlockTXLimit() uint64 {
	l.RLock()
	t := c.blockTxLimit
	l.RUnlock()

	return t
}

func GetMissingTxPullLimit() uint64 {
	l.RLock()
	t := c.missingTxPullLimit
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

func Reset() {
	l.Lock()
	c = defaultConf
	l.Unlock()
}
