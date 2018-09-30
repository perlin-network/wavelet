package stats

import (
	"expvar"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
)

var (
	numAcceptedTransactions          = expvar.NewInt("perlin_num_accepted_transactions")
	numAcceptedTransactionsPerSecond = expvar.NewInt("perlin_num_accepted_transactions_per_sec")
	consensusDuration                = expvar.NewFloat("perlin_consensus_duration")

	uptime  = expvar.NewString("perlin_uptime")
	laptime = expvar.NewFloat("perlin_laptime")

	bufferAcceptByTagPerSec = expvar.NewMap("perlin_buffer_accept_by_tag_per_sec")
	lastAcceptByTagPerSec   = expvar.NewMap("perlin_last_accept_by_tag_per_sec")

	startTime    time.Time
	lapStartTime time.Time
)

func init() {
	Reset()

	startTime = time.Now()

	go func() {
		for range time.Tick(1 * time.Second) {
			numAcceptedTransactionsPerSecond.Set(0)
			uptime.Set(time.Now().Sub(startTime).String())
			laptime.Set(time.Now().Sub(lapStartTime).Seconds())

			lastAcceptByTagPerSec.Init()

			bufferAcceptByTagPerSec.Do(func(kv expvar.KeyValue) {
				lastAcceptByTagPerSec.Set(kv.Key, kv.Value)
			})

			bufferAcceptByTagPerSec.Init()
		}
	}()

	expvar.Publish("system_stats", expvar.Func(SystemInfo))
}

// IncAcceptedTransactions will increment #accepted transactions for the tag
func IncAcceptedTransactions(tag string) {
	numAcceptedTransactions.Add(1)
	numAcceptedTransactionsPerSecond.Add(1)
	bufferAcceptByTagPerSec.Add(tag, 1)
	bufferAcceptByTagPerSec.Add("total", 1)
}

// DecAcceptedTransactions will decrement #accepted transactions by 1.
func DecAcceptedTransactions() {
	numAcceptedTransactions.Set(numAcceptedTransactions.Value() - 1)
}

// SetConsensusDuration will update last consensus duration.
func SetConsensusDuration(value float64) {
	consensusDuration.Set(value)
}

// Reset sets all metrics to 0
func Reset() {
	lapStartTime = time.Now()
	consensusDuration.Set(0)
	numAcceptedTransactions.Set(0)
	numAcceptedTransactionsPerSecond.Set(0)
	lastAcceptByTagPerSec.Init()
	bufferAcceptByTagPerSec.Init()
}

// Summary returns a summary of the stats
func Summary() interface{} {
	t, _ := time.ParseDuration(uptime.Value())

	acceptByTagPerSec := make(map[string]int64)
	lastAcceptByTagPerSec.Do(func(kv expvar.KeyValue) {
		if iv, ok := kv.Value.(*expvar.Int); ok {
			acceptByTagPerSec[kv.Key] = iv.Value()
		}
	})

	summary := struct {
		ConsensusDuration                float64          `json:"consensus_duration"`
		NumAcceptedTransactions          int64            `json:"num_accepted_tx"`
		NumAcceptedTransactionsPerSecond int64            `json:"num_accepted_tx_per_sec"`
		Uptime                           float64          `json:"uptime"`
		LapTime                          float64          `json:"laptime"`
		AcceptByTagPerSec                map[string]int64 `json:"accept_by_tag_per_sec"`
	}{
		ConsensusDuration:                consensusDuration.Value(),
		NumAcceptedTransactions:          numAcceptedTransactions.Value(),
		NumAcceptedTransactionsPerSecond: numAcceptedTransactionsPerSecond.Value(),
		Uptime:                           t.Seconds(),
		LapTime:                          laptime.Value(),
		AcceptByTagPerSec:                acceptByTagPerSec,
	}

	return summary
}

// SystemInfo returns real-time information about the current system.
//
// TODO: Have system information be cross-platform. Linux only for now.
func SystemInfo() interface{} {
	c, _ := cpu.Percent(time.Second, true)

	diskDeviceName := "/dev/sda1"

	d, _ := disk.IOCounters(diskDeviceName)
	v, _ := mem.VirtualMemory()

	systemStats := struct {
		Memory *mem.VirtualMemoryStat `json:"memory"`
		CPU    []float64              `json:"cpu"`
		Disk   disk.IOCountersStat    `json:"disk"`
	}{
		CPU:    c,
		Disk:   d[diskDeviceName],
		Memory: v,
	}

	return systemStats
}
