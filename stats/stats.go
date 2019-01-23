package stats

import (
	"expvar"
	"fmt"
	"time"
)

var (
	numAcceptedTransactionsStat          expvar.Int
	numAcceptedTransactionsPerSecondStat expvar.Int
	consensusDurationStat                expvar.Float
	numPendingTx                         expvar.Int
	numConnectedPeers                    expvar.Int

	uptimeStat  expvar.String
	laptimeStat expvar.Float

	bufferAcceptByTagPerSecStat expvar.Map
	lastAcceptByTagPerSecStat   expvar.Map

	bufferMiscPerSecStat expvar.Map
	lastMiscPerSecStat   expvar.Map

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

			// roll the map stats
			rollStatMap(&bufferAcceptByTagPerSecStat, &lastAcceptByTagPerSecStat)
			rollStatMap(&bufferMiscPerSecStat, &lastMiscPerSecStat)
		}
	}()

	// publish the custom struct
	expvar.Publish("wavelet", expvar.Func(Summary))
}

// IncAcceptedTransactions will increment #accepted transactions for the tag
func IncAcceptedTransactions(tag uint32) {
	numAcceptedTransactionsStat.Add(1)
	numAcceptedTransactionsPerSecondStat.Add(1)
	bufferAcceptByTagPerSecStat.Add(fmt.Sprintf("%d", tag), 1)
	bufferAcceptByTagPerSecStat.Add("total", 1)
}

// DecAcceptedTransactions will decrement #accepted transactions by 1.
func DecAcceptedTransactions() {
	numAcceptedTransactionsStat.Set(numAcceptedTransactionsStat.Value() - 1)
}

// IncMiscPerSecStat will increment a misc tag that is reset every second
func IncMiscPerSecStat(tag string, amount int) {
	bufferMiscPerSecStat.Add(tag, int64(amount))
}

// SetConsensusDuration will update last consensus duration.
func SetConsensusDuration(value float64) {
	consensusDurationStat.Set(value)
}

// SetNumPendingTx will update the number of pending transactions.
func SetNumPendingTx(value int64) {
	numPendingTx.Set(value)
}

// SetNumConnectedPeers will update the number of connected peers.
func SetNumConnectedPeers(value int64) {
	numConnectedPeers.Set(value)
}

// Reset sets all metrics to 0
func Reset() {
	lapStartTime = time.Now()
	consensusDurationStat.Set(0)
	numPendingTx.Set(0)
	numAcceptedTransactionsStat.Set(0)
	numAcceptedTransactionsPerSecondStat.Set(0)
	numConnectedPeers.Set(0)
	lastAcceptByTagPerSecStat.Init()
	bufferAcceptByTagPerSecStat.Init()
	lastMiscPerSecStat.Init()
	bufferMiscPerSecStat.Init()
}

// Summary returns a custom summary struct
func Summary() interface{} {
	t, _ := time.ParseDuration(uptimeStat.Value())

	summary := struct {
		ConsensusDuration                float64          `json:"consensus_duration"`
		NumAcceptedTransactions          int64            `json:"num_accepted_transactions"`
		NumAcceptedTransactionsPerSecond int64            `json:"num_accepted_transactions_per_sec"`
		NumPendingTx                     int64            `json:"num_pending_transactions"`
		NumConnectedPeers                int64            `json:"num_connected_peers"`
		Uptime                           float64          `json:"uptime"`
		LapTime                          float64          `json:"laptime"`
		LastAcceptByTagPerSec            map[string]int64 `json:"last_accept_by_tag_per_sec"`
		BufferAcceptByTagPerSec          map[string]int64 `json:"buffer_accept_by_tag_per_sec"`
		LastMiscPerSec                   map[string]int64 `json:"last_misc_per_sec"`
		BufferMiscPerSec                 map[string]int64 `json:"buffer_misc_per_sec"`
	}{
		ConsensusDuration:                consensusDurationStat.Value(),
		NumAcceptedTransactions:          numAcceptedTransactionsStat.Value(),
		NumAcceptedTransactionsPerSecond: numAcceptedTransactionsPerSecondStat.Value(),
		NumPendingTx:                     numPendingTx.Value(),
		NumConnectedPeers:                numConnectedPeers.Value(),
		Uptime:                           t.Seconds(),
		LapTime:                          laptimeStat.Value(),
		LastAcceptByTagPerSec:            convertMap(&lastAcceptByTagPerSecStat),
		BufferAcceptByTagPerSec:          convertMap(&bufferAcceptByTagPerSecStat),
		LastMiscPerSec:                   convertMap(&lastMiscPerSecStat),
		BufferMiscPerSec:                 convertMap(&bufferMiscPerSecStat),
	}

	return summary
}

// convertMap converts expvar.Map into map[string]int64 for printing
func convertMap(src *expvar.Map) map[string]int64 {
	dst := make(map[string]int64)
	src.Do(func(kv expvar.KeyValue) {
		if iv, ok := kv.Value.(*expvar.Int); ok {
			dst[kv.Key] = iv.Value()
		}
	})
	return dst
}

// rollStatMap copies the values from buffer and puts them into last, and resets the buffer
func rollStatMap(buffer *expvar.Map, last *expvar.Map) {
	// reset the last stats
	last.Init()

	// copy over the buffered stats
	buffer.Do(func(kv expvar.KeyValue) {
		last.Set(kv.Key, kv.Value)
	})

	// reset the buffered stats
	buffer.Init()
}
