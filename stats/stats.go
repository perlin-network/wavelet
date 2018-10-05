package stats

import (
	"expvar"
	"time"
)

var (
	numAcceptedTransactionsStat          expvar.Int
	numAcceptedTransactionsPerSecondStat expvar.Int
	consensusDurationStat                expvar.Float

	uptimeStat  expvar.String
	laptimeStat expvar.Float

	bufferAcceptByTagPerSecStat expvar.Map
	lastAcceptByTagPerSecStat   expvar.Map

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

			// reset the last stats
			lastAcceptByTagPerSecStat.Init()

			// copy over the buffered stats
			bufferAcceptByTagPerSecStat.Do(func(kv expvar.KeyValue) {
				lastAcceptByTagPerSecStat.Set(kv.Key, kv.Value)
			})

			// reset the buffered stats
			bufferAcceptByTagPerSecStat.Init()
		}
	}()

	// publish the custom struct
	expvar.Publish("wavelet", expvar.Func(Summary))
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
}

// Summary returns a custom summary struct
func Summary() interface{} {
	t, _ := time.ParseDuration(uptimeStat.Value())

	lastAcceptByTagPerSec := make(map[string]int64)
	lastAcceptByTagPerSecStat.Do(func(kv expvar.KeyValue) {
		if iv, ok := kv.Value.(*expvar.Int); ok {
			lastAcceptByTagPerSec[kv.Key] = iv.Value()
		}
	})

	bufferAcceptByTagPerSec := make(map[string]int64)
	bufferAcceptByTagPerSecStat.Do(func(kv expvar.KeyValue) {
		if iv, ok := kv.Value.(*expvar.Int); ok {
			bufferAcceptByTagPerSec[kv.Key] = iv.Value()
		}
	})

	summary := struct {
		ConsensusDuration                float64          `json:"consensus_duration"`
		NumAcceptedTransactions          int64            `json:"num_accepted_transactions"`
		NumAcceptedTransactionsPerSecond int64            `json:"num_accepted_transactions_per_sec"`
		Uptime                           float64          `json:"uptime"`
		LapTime                          float64          `json:"laptime"`
		LastAcceptByTagPerSec            map[string]int64 `json:"last_accept_by_tag_per_sec"`
		BufferAcceptByTagPerSec          map[string]int64 `json:"buffer_accept_by_tag_per_sec"`
	}{
		ConsensusDuration:                consensusDurationStat.Value(),
		NumAcceptedTransactions:          numAcceptedTransactionsStat.Value(),
		NumAcceptedTransactionsPerSecond: numAcceptedTransactionsPerSecondStat.Value(),
		Uptime:                           t.Seconds(),
		LapTime:                          laptimeStat.Value(),
		LastAcceptByTagPerSec:            lastAcceptByTagPerSec,
		BufferAcceptByTagPerSec:          bufferAcceptByTagPerSec,
	}

	return summary
}
