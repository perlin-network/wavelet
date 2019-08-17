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

	queried         metrics.Meter
	queriedPerRound metrics.Gauge

	gossipedTX   metrics.Meter
	receivedTX   metrics.Meter
	acceptedTX   metrics.Meter
	downloadedTX metrics.Meter

	collapseLatency     metrics.Timer
	finalizationLatency metrics.Timer
	queryLatency        metrics.Timer
}

func NewMetrics(ctx context.Context) *Metrics {
	registry := metrics.NewRegistry()

	queried := metrics.NewRegisteredMeter("round.queried.total", registry)
	queriedPerRound := metrics.NewRegisteredGauge("round.queried.per_round", registry)

	gossipedTX := metrics.NewRegisteredMeter("tx.gossiped", registry)
	receivedTX := metrics.NewRegisteredMeter("tx.received", registry)
	acceptedTX := metrics.NewRegisteredMeter("tx.accepted", registry)
	downloadedTX := metrics.NewRegisteredMeter("tx.downloaded", registry)

	collapseLatency := metrics.NewRegisteredTimer("collapse.latency", registry)
	finalizationLatency := metrics.NewRegisteredTimer("finalization.latency", registry)
	queryLatency := metrics.NewRegisteredTimer("query.latency", registry)

	go func() {
		logger := log.Metrics()

		for {
			select {
			case <-time.After(1 * time.Second):
				logger.Info().
					Int64("round.queried.total", queried.Count()).
					Int64("round.queried.per_round", queriedPerRound.Value()).
					Int64("tx.gossiped", gossipedTX.Count()).
					Int64("tx.received", receivedTX.Count()).
					Int64("tx.accepted", acceptedTX.Count()).
					Int64("tx.downloaded", downloadedTX.Count()).
					Float64("rps.queried", queried.Rate1()).
					Float64("tps.gossiped", gossipedTX.Rate1()).
					Float64("tps.received", receivedTX.Rate1()).
					Float64("tps.accepted", acceptedTX.Rate1()).
					Float64("tps.downloaded", downloadedTX.Rate1()).
					Str("query.latency.max.ms", time.Duration(queryLatency.Max()).String()).
					Str("query.latency.min.ms", time.Duration(queryLatency.Min()).String()).
					Str("query.latency.mean.ms", time.Duration(queryLatency.Rate1()).String()).
					Str("finalization.latency.max.ms", time.Duration(finalizationLatency.Max()).String()).
					Str("finalization.latency.min.ms", time.Duration(finalizationLatency.Min()).String()).
					Str("finalization.latency.mean.ms", time.Duration(finalizationLatency.Rate1()).String()).
					Str("collapse.latency.max.ms", time.Duration(collapseLatency.Max()).String()).
					Str("collapse.latency.min.ms", time.Duration(collapseLatency.Min()).String()).
					Str("collapse.latency.mean.ms", time.Duration(collapseLatency.Rate1()).String()).
					Msg("Updated metrics.")
			case <-ctx.Done():
				return
			}
		}
	}()

	return &Metrics{
		registry: registry,

		queried:         queried,
		queriedPerRound: queriedPerRound,

		gossipedTX:   gossipedTX,
		receivedTX:   receivedTX,
		acceptedTX:   acceptedTX,
		downloadedTX: downloadedTX,

		collapseLatency:     collapseLatency,
		finalizationLatency: finalizationLatency,
		queryLatency:        queryLatency,
	}
}

func (m *Metrics) Stop() {
	m.queried.Stop()

	m.gossipedTX.Stop()
	m.receivedTX.Stop()
	m.acceptedTX.Stop()
	m.downloadedTX.Stop()

	m.collapseLatency.Stop()
	m.finalizationLatency.Stop()
	m.queryLatency.Stop()
}
