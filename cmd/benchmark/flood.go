// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package main

import (
	"runtime"
	"sync"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
)

type floodFunc func(client *wctl.Client) ([]wctl.SendTransactionResponse, error)

func floodBatchStake() floodFunc {
	return flood(sys.TagBatch, func(i int) []byte {
		n := 40
		payload := wavelet.Batch{
			Tags:     make([]uint8, 0, n),
			Payloads: make([][]byte, 0, n),
		}

		stake := wavelet.Stake{Opcode: sys.PlaceStake, Amount: uint64(i + 1)}
		for i := 0; i < 40; i++ {
			if err := payload.AddStake(stake); err != nil {
				// Shouldn't happen
				panic(err)
			}
		}

		return payload.Marshal()
	})
}

func floodPayload(payload []byte) floodFunc {
	return flood(sys.TagTransfer, func(i int) []byte {
		return payload
	})
}

func floodContracts(code []byte) floodFunc {
	return flood(sys.TagContract, func(i int) []byte {
		payload := wavelet.Contract{
			GasLimit: 100000000,
			Code:     code,
		}

		return payload.Marshal()
	})
}

func flood(tag sys.Tag, getPayload func(i int) []byte) floodFunc {
	return func(client *wctl.Client) ([]wctl.SendTransactionResponse, error) {
		numWorkers := runtime.NumCPU()

		var wg sync.WaitGroup
		wg.Add(numWorkers)

		chRes := make(chan wctl.SendTransactionResponse, numWorkers)
		chErr := make(chan error, numWorkers)

		for i := 0; i < numWorkers; i++ {
			go func(i int) {
				var res wctl.SendTransactionResponse
				res, err := client.SendTransaction(byte(tag), getPayload(i))
				chRes <- res
				chErr <- err

				wg.Done()
			}(i)
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
