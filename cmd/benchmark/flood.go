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

				for i := 0; i < 40; i++ {
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
