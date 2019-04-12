package main

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"go.uber.org/atomic"
	"runtime"
	"sync"
	"time"
)

func floodTransactions() func(client *wctl.Client) ([]wctl.SendTransactionResponse, error) {
	tps := atomic.NewUint64(0)

	go func() {
		for range time.Tick(1 * time.Second) {
			log.Info().Uint64("tps", tps.Swap(0)).Msg("Benchmarking...")
		}
	}()

	return func(client *wctl.Client) ([]wctl.SendTransactionResponse, error) {
		numWorkers := runtime.NumCPU()

		var wg sync.WaitGroup
		wg.Add(numWorkers)

		chRes := make(chan wctl.SendTransactionResponse, numWorkers)
		chErr := make(chan error, numWorkers)

		for i := 0; i < numWorkers; i++ {
			i := i + 1

			go func() {
				defer wg.Done()

				var payload [8]byte
				binary.LittleEndian.PutUint64(payload[:8], uint64(i))

				var res wctl.SendTransactionResponse

				res, err := client.SendTransaction(sys.TagStake, payload[:])

				if err != nil {
					chRes <- res
					chErr <- err
				}

				chRes <- res
				chErr <- nil
			}()
		}

		wg.Wait()

		var responses []wctl.SendTransactionResponse
		var err error

		for i := 0; i < numWorkers; i++ {
			if e := <-chErr; err == nil {
				err = e
			} else {
				err = errors.Wrap(err, e.Error())
			}

			responses = append(responses, <-chRes)
		}

		tps.Add(uint64(numWorkers))

		return responses, nil
	}
}
