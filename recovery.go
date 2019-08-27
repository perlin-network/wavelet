package wavelet

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/lru"
	"github.com/pkg/errors"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
)

type StallDetector struct {
	mu       sync.Mutex
	stop     <-chan struct{}
	config   StallDetectorConfig
	delegate StallDetectorDelegate

	// Network statistics.
	lastNetworkActivityTime time.Time

	// Round statistics.
	roundSet             *lru.LRU // RoundID -> struct{}
	lastRoundTime        time.Time
	lastFinalizationTime time.Time
}

type StallDetectorConfig struct {
	MaxMemoryMB uint64
}

type StallDetectorDelegate struct {
	Ping            func()
	PrepareShutdown func(error)
}

func NewStallDetector(stop <-chan struct{}, config StallDetectorConfig, delegate StallDetectorDelegate) *StallDetector {
	return &StallDetector{
		stop:                    stop,
		config:                  config,
		delegate:                delegate,
		lastNetworkActivityTime: time.Now(),
		roundSet:                lru.NewLRU(4),
		lastRoundTime:           time.Now(),
		lastFinalizationTime:    time.Now(),
	}
}

func (d *StallDetector) Run() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	logger := log.Node()

	for {
		select {
		case <-ticker.C:
			d.mu.Lock()
			currentTime := time.Now()

			hasNetworkActivityRecently := false

			if currentTime.After(d.lastNetworkActivityTime) {
				if currentTime.Sub(d.lastNetworkActivityTime) > 120*time.Second {
					d.delegate.PrepareShutdown(errors.New("We did not detect any network activity during the last 2 minutes, and our Ping requests have got no responses. Node is scheduled to shutdown now."))
					restartErr := d.tryRestart()
					logger.Error().Err(restartErr).Msg("Failed to restart process")
					d.mu.Unlock()
					return // restarting process is impossible. No longer run the stall detector.
				} else if currentTime.Sub(d.lastNetworkActivityTime) > 60*time.Second {
					d.delegate.Ping()
				} else {
					hasNetworkActivityRecently = true
				}
			}
			if currentTime.After(d.lastFinalizationTime) && currentTime.After(d.lastRoundTime) && currentTime.Sub(d.lastRoundTime) > 180*time.Second && hasNetworkActivityRecently {
				d.delegate.PrepareShutdown(errors.New("Seems that consensus has stalled. Node is scheduled to shutdown now."))
				restartErr := d.tryRestart()
				logger.Error().Err(restartErr).Msg("Failed to restart process")
				d.mu.Unlock()
				return
			}
			if d.config.MaxMemoryMB > 0 {
				var memStats runtime.MemStats
				runtime.ReadMemStats(&memStats)
				if memStats.Alloc > 1048576*d.config.MaxMemoryMB {
					d.delegate.PrepareShutdown(errors.New("Memory usage exceeded maximum. Node is scheduled to shutdown now."))
					restartErr := d.tryRestart()

					errorLog := log.Node()
					pprof.Lookup("heap").WriteTo(errorLog, 2)
					pprof.Lookup("goroutine").WriteTo(errorLog, 2)

					logger.Error().Err(restartErr).Msg("Failed to restart process")
					d.mu.Unlock()
					return
				}
			}

			d.mu.Unlock()
		case <-d.stop:
		}
	}
}

func (d *StallDetector) ReportNetworkActivity() {
	d.mu.Lock()
	d.lastNetworkActivityTime = time.Now()
	d.mu.Unlock()
}

func (d *StallDetector) ReportIncomingRound(roundID RoundID) {
	d.mu.Lock()
	if d.roundSet != nil {
		if _, loaded := d.roundSet.LoadOrPut(roundID, struct{}{}); !loaded {
			d.lastRoundTime = time.Now()
		}
	}
	d.mu.Unlock()
}

func (d *StallDetector) ReportFinalizedRound(roundID RoundID) {
	d.mu.Lock()
	d.roundSet = lru.NewLRU(4)
	d.lastRoundTime = time.Now()
	d.mu.Unlock()
}
