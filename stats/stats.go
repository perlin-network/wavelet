package stats

import (
	"expvar"
	"time"
)

const (
	numAcceptedTransactions          = "num_accepted_transactions"
	numAcceptedTransactionsPerSecond = "num_accepted_transactions_per_sec"
	consensusDuration                = "consensus_duration"

	uptime  = "uptime"
	laptime = "laptime"

	bufferAcceptByTagPerSec = "buffer_accept_by_tag_per_sec"
	lastAcceptByTagPerSec   = "last_accept_by_tag_per_sec"
)

var (
	numAcceptedTransactionsStat          expvar.Int
	numAcceptedTransactionsPerSecondStat expvar.Int
	consensusDurationStat                expvar.Float

	uptimeStat  expvar.String
	laptimeStat expvar.Float

	bufferAcceptByTagPerSecStat expvar.Map
	lastAcceptByTagPerSecStat   expvar.Map

	stats = expvar.NewMap("wavelet")

	startTime    time.Time
	lapStartTime time.Time
)

func init() {
	Reset()

	startTime = time.Now()

	go func() {
		for range time.Tick(1 * time.Second) {
			numAcceptedTransactionsPerSecondStat.Set(0)
			uptimeStat.Set(time.Now().Sub(startTime).String())
			laptimeStat.Set(time.Now().Sub(lapStartTime).Seconds())

			lastAcceptByTagPerSecStat.Init()

			bufferAcceptByTagPerSecStat.Do(func(kv expvar.KeyValue) {
				lastAcceptByTagPerSecStat.Set(kv.Key, kv.Value)
			})

			bufferAcceptByTagPerSecStat.Init()
		}
	}()
}

// IncAcceptedTransactions will increment #accepted transactions for the tag
func IncAcceptedTransactions(tag string) {
	numAcceptedTransactionsStat.Add(1)
	numAcceptedTransactionsPerSecondStat.Add(1)
	bufferAcceptByTagPerSecStat.Add(tag, 1)
	bufferAcceptByTagPerSecStat.Add("total", 1)
}

// DecAcceptedTransactions will decrement #accepted transactions by 1.
func DecAcceptedTransactions() {
	numAcceptedTransactionsStat.Set(numAcceptedTransactionsStat.Value() - 1)
}

// SetConsensusDuration will update last consensus duration.
func SetConsensusDuration(value float64) {
	consensusDurationStat.Set(value)
}

// Reset sets all metrics to 0
func Reset() {
	lapStartTime = time.Now()
	consensusDurationStat.Set(0)
	numAcceptedTransactionsStat.Set(0)
	numAcceptedTransactionsPerSecondStat.Set(0)
	lastAcceptByTagPerSecStat.Init()
	bufferAcceptByTagPerSecStat.Init()

	stats.Init()
	stats.Set(numAcceptedTransactions, &numAcceptedTransactionsStat)
	stats.Set(numAcceptedTransactionsPerSecond, &numAcceptedTransactionsPerSecondStat)
	stats.Set(consensusDuration, &consensusDurationStat)
	stats.Set(uptime, &uptimeStat)
	stats.Set(laptime, &laptimeStat)
	stats.Set(bufferAcceptByTagPerSec, &bufferAcceptByTagPerSecStat)
	stats.Set(lastAcceptByTagPerSec, &lastAcceptByTagPerSecStat)
}
