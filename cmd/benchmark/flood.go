package main

import (
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/rs/zerolog/log"
	"go.uber.org/atomic"
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
		res, err := client.SendTransaction(sys.TagStake, payload.NewWriter(nil).WriteUint64(1).Bytes())
		if err != nil {
			return nil, err
		}

		tps.Add(1)

		return []wctl.SendTransactionResponse{res}, nil

		//numWorkers := runtime.NumCPU()
		//
		//var wg sync.WaitGroup
		//wg.Add(numWorkers)
		//
		//chRes := make(chan wctl.SendTransactionResponse, numWorkers)
		//chErr := make(chan error, numWorkers)
		//
		//for i := 0; i < numWorkers; i++ {
		//	go func() {
		//		defer wg.Done()
		//
		//		res, err := client.SendTransaction(sys.TagStake, payload.NewWriter(nil).WriteUint64(1).Bytes())
		//		if err != nil {
		//			chRes <- res
		//			chErr <- err
		//		}
		//
		//		chRes <- res
		//		chErr <- nil
		//	}()
		//}
		//
		//wg.Wait()
		//
		//var responses []wctl.SendTransactionResponse
		//var err error
		//
		//for i := 0; i < numWorkers; i++ {
		//	if e := <-chErr; err == nil {
		//		err = e
		//	} else {
		//		err = errors.Wrap(err, e.Error())
		//	}
		//
		//	responses = append(responses, <-chRes)
		//}

		//return responses, nil
	}
}
