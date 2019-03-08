package main

import (
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/rs/zerolog/log"
	"go.uber.org/atomic"
	"time"
)

func floodTransactions() func(client *wctl.Client) (wctl.SendTransactionResponse, error) {
	tps := atomic.NewUint64(0)

	go func() {
		for range time.Tick(1 * time.Second) {
			log.Info().Uint64("tps", tps.Swap(0)).Msg("Benchmarking...")
		}
	}()

	return func(client *wctl.Client) (wctl.SendTransactionResponse, error) {
		res, err := client.SendTransaction(sys.TagStake, payload.NewWriter(nil).WriteUint64(1).Bytes())
		if err != nil {
			return res, err
		}

		tps.Add(1)

		return res, nil
	}
}
