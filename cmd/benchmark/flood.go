package main

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
	"runtime"
	"sync"
)

func floodTransactions() func(client *wctl.Client) ([]wctl.SendTransactionResponse, error) {
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

				var base [9]byte

				base[0] = 1
				binary.LittleEndian.PutUint64(base[1:9], uint64(i))

				var tags []byte
				var payloads [][]byte

				for i := 0; i < 10; i++ {
					tags = append(tags, sys.TagStake)
					payloads = append(payloads, base[:])
				}

				var size [4]byte
				var buf []byte

				for i := range tags {
					buf = append(buf, tags[i])

					binary.BigEndian.PutUint32(size[:4], uint32(len(payloads[i])))
					buf = append(buf, size[:4]...)
					buf = append(buf, payloads[i]...)
				}

				var res wctl.SendTransactionResponse

				res, err := client.SendTransaction(sys.TagBatch, append([]byte{byte(len(tags))}, buf...))

				if err != nil {
					chRes <- res
					chErr <- err

					return
				}

				chRes <- res
				chErr <- err
			}()
		}

		wg.Wait()

		var responses []wctl.SendTransactionResponse
		var err error

		for i := 0; i < numWorkers; i++ {
			if e := <-chErr; e != nil {
				if err == nil {
					err = e
				} else {
					err = errors.Wrap(err, e.Error())
				}
			}

			responses = append(responses, <-chRes)
		}

		return responses, err
	}
}
