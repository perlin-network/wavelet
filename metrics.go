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

package wavelet

import (
	"context"
	"github.com/perlin-network/wavelet/log"
	"github.com/rcrowley/go-metrics"
	"time"
)

type Metrics struct {
	registry metrics.Registry

	queried metrics.Meter

	gossipedTX   metrics.Meter
	receivedTX   metrics.Meter
	acceptedTX   metrics.Meter
	downloadedTX metrics.Meter

	queryLatency metrics.Timer
}

func NewMetrics(ctx context.Context) *Metrics {
	registry := metrics.NewRegistry()

	queried := metrics.NewRegisteredMeter("round.queried", registry)

	gossipedTX := metrics.NewRegisteredMeter("tx.gossiped", registry)
	receivedTX := metrics.NewRegisteredMeter("tx.received", registry)
	acceptedTX := metrics.NewRegisteredMeter("tx.accepted", registry)
	downloadedTX := metrics.NewRegisteredMeter("tx.downloaded", registry)

	queryLatency := metrics.NewRegisteredTimer("query.latency", registry)

	go func() {
		logger := log.Metrics()

		for {
			select {
			case <-time.After(1 * time.Second):
				logger.Info().
					Int64("round.queried", queried.Count()).
					Int64("tx.gossiped", gossipedTX.Count()).
					Int64("tx.received", receivedTX.Count()).
					Int64("tx.accepted", acceptedTX.Count()).
					Int64("tx.downloaded", downloadedTX.Count()).
					Float64("rps.queried", queried.RateMean()).
					Float64("tps.gossiped", gossipedTX.RateMean()).
					Float64("tps.received", receivedTX.RateMean()).
					Float64("tps.accepted", acceptedTX.RateMean()).
					Float64("tps.downloaded", downloadedTX.RateMean()).
					Int64("query.latency.max.ms", queryLatency.Max()/(1.0e+7)).
					Int64("query.latency.min.ms", queryLatency.Min()/(1.0e+7)).
					Float64("query.latency.mean.ms", queryLatency.Mean()/(1.0e+7)).
					Msg("Updated metrics.")
			case <-ctx.Done():
				return
			}
		}
	}()

	return &Metrics{
		registry: registry,

		queried: queried,

		gossipedTX:   gossipedTX,
		receivedTX:   receivedTX,
		acceptedTX:   acceptedTX,
		downloadedTX: downloadedTX,

		queryLatency: queryLatency,
	}
}

func (m *Metrics) Stop() {
	m.queried.Stop()

	m.gossipedTX.Stop()
	m.receivedTX.Stop()
	m.acceptedTX.Stop()
	m.downloadedTX.Stop()

	m.queryLatency.Stop()
}
