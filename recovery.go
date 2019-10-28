package wavelet

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
)

type StallDetector struct {
	mu       *sync.Mutex
	stop     chan struct{}
	config   StallDetectorConfig
	delegate StallDetectorDelegate
}

type StallDetectorConfig struct {
	MaxMemoryMB uint64
}

type StallDetectorDelegate struct {
	PrepareShutdown func(error)
}

func (d StallDetectorDelegate) prepareShutdown(mu *sync.Mutex, err error) {
	mu.Unlock()
	d.PrepareShutdown(err)
	mu.Lock()
}

func NewStallDetector(config StallDetectorConfig, delegate StallDetectorDelegate) *StallDetector {
	return &StallDetector{
		mu:       &sync.Mutex{},
		stop:     make(chan struct{}),
		config:   config,
		delegate: delegate,
	}
}

func (d *StallDetector) Stop() {
	close(d.stop)
}

func (d *StallDetector) Run(wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	logger := log.Node()

LOOP:
	for {
		select {
		case <-ticker.C:
			func() {
				d.mu.Lock()
				defer d.mu.Unlock()

				if d.config.MaxMemoryMB > 0 {
					var memStats runtime.MemStats
					runtime.ReadMemStats(&memStats)

					if memStats.Alloc > 1048576*d.config.MaxMemoryMB {
						d.delegate.prepareShutdown(d.mu, errors.New("Memory usage exceeded maximum. Node is scheduled to shutdown now."))

						func() {
							// Create directory where we will store the dump
							// XXX:TODO: Make this user servicable
							crashDir := "./crashes"
							if err := os.MkdirAll(crashDir, 0700); err != nil {
								logger.Error().Err(err).Msgf("Failed to create a directory to record crash logs in: %v", crashDir)

								return
							}

							// Write out two logs: 1. heap; 2. goroutine, with the
							// timestamp embedded
							crashTimestamp := time.Now().Format("2006-01-02-15-04")
							heapFileName := fmt.Sprintf("%s/heap-%s.pprof", crashDir, crashTimestamp)
							stackFileName := fmt.Sprintf("%s/goroutine-%s.pprof", crashDir, crashTimestamp)

							// Write out the heap dump
							heapFile, err := os.Create(heapFileName)
							if err != nil {
								logger.Error().Err(err).Msgf("Failed to create pprof file: %v", heapFileName)
								return
							}

							defer func() {
								_ = heapFile.Close()
							}()

							err = pprof.Lookup("heap").WriteTo(heapFile, 0)
							if err != nil {
								logger.Error().Err(err).Msgf("Failed to write pprof file: %v", heapFileName)
								return
							}

							// Write out the goroutine stack dump
							stackFile, err := os.Create(stackFileName)
							if err != nil {
								logger.Error().Err(err).Msgf("Failed to create goroutines stack dump file: %v", stackFileName)
								return
							}

							defer func() {
								_ = stackFile.Close()
							}()

							err = pprof.Lookup("goroutine").WriteTo(stackFile, 0)
							if err != nil {
								logger.Error().Err(err).Msgf("Failed to write goroutines stack dump file: %v", stackFileName)

								return
							}
						}()

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
