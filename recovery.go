package wavelet

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/lru"
	"github.com/pkg/errors"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
	"fmt"
	"os"
)

type StallDetector struct {
	mu       *sync.Mutex
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

func (d StallDetectorDelegate) ping(mu *sync.Mutex) {
	mu.Unlock()
	d.Ping()
	mu.Lock()
}

func (d StallDetectorDelegate) prepareShutdown(mu *sync.Mutex, err error) {
	mu.Unlock()
	d.PrepareShutdown(err)
	mu.Lock()
}

func NewStallDetector(stop <-chan struct{}, config StallDetectorConfig, delegate StallDetectorDelegate) *StallDetector {
	return &StallDetector{
		mu:                      &sync.Mutex{},
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

LOOP:
	for {
		select {
		case <-ticker.C:
			currentTime := time.Now()

			hasNetworkActivityRecently := false

			func() {
				d.mu.Lock()
				defer d.mu.Unlock()

				if currentTime.After(d.lastNetworkActivityTime) {
					if currentTime.Sub(d.lastNetworkActivityTime) > 120*time.Second {
						d.delegate.prepareShutdown(d.mu, errors.New("We did not detect any network activity during the last 2 minutes, and our Ping requests have got no responses. Node is scheduled to shutdown now."))

						if err := d.tryRestart(); err != nil {
							logger.Error().Err(err).Msg("Failed to restart process")
						}

						return // Restarting process is impossible. Stop running the stall detector.
					} else if currentTime.Sub(d.lastNetworkActivityTime) > 60*time.Second {
						d.delegate.ping(d.mu)
					} else {
						hasNetworkActivityRecently = true
					}
				}

				if currentTime.After(d.lastFinalizationTime) && currentTime.After(d.lastRoundTime) && currentTime.Sub(d.lastRoundTime) > 180*time.Second && hasNetworkActivityRecently {
					d.delegate.prepareShutdown(d.mu, errors.New("Seems that consensus has stalled. Node is scheduled to shutdown now."))

					if err := d.tryRestart(); err != nil {
						logger.Error().Err(err).Msg("Failed to restart process")
					}

					return
				}

				if d.config.MaxMemoryMB > 0 {
					var memStats runtime.MemStats
					runtime.ReadMemStats(&memStats)

					if memStats.Alloc > 1048576*d.config.MaxMemoryMB {
						d.delegate.prepareShutdown(d.mu, errors.New("Memory usage exceeded maximum. Node is scheduled to shutdown now."))

						logger := log.Node()
						_ = pprof.Lookup("heap").WriteTo(logger, 2)
						_ = pprof.Lookup("goroutine").WriteTo(logger, 2)

						crashDir := "./crashes"
						if err := os.MkdirAll(crashDir, 0700); err == nil {
							crashTimestamp := time.Now().Format("2006-01-02-15-04")
							crashFile := fmt.Sprintf("%s/heap-%s.pprof", crashDir, crashTimestamp)

							if heapFile, err := os.Create(crashFile); err == nil {
								_ = pprof.Lookup("heap").WriteTo(heapFile, 0)
								heapFile.Close()
							}
						}

						if err := d.tryRestart(); err != nil {
							logger.Error().Err(err).Msg("Failed to restart process")
						}

						return
					}
				}
			}()
		case <-d.stop:
			break LOOP
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
