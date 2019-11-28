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
	"time"

	"github.com/perlin-network/wavelet/log"
	"github.com/rcrowley/go-metrics"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

type MetricsService struct {
	registry metrics.Registry
	context  context.Context

	ticker *time.Ticker

	queried metrics.Meter

	gossipedTX   metrics.Meter
	receivedTX   metrics.Meter
	acceptedTX   metrics.Meter
	downloadedTX metrics.Meter

	finalizedBlocks metrics.Meter

	queryLatency metrics.Timer

	LastMetrics *Metrics
	LastPolled  time.Time
}

type Metrics struct {
	BlocksQueried int64 `json:"blocks.queried"`

	TxGossiped   int64 `json:"tx.gossiped"`
	TxReceived   int64 `json:"tx.received"`
	TxAccepted   int64 `json:"tx.accepted"`
	TxDownloaded int64 `json:"tx.downloaded"`

	BPSQueried      float64 `json:"bps.queried"`
	TPSGossiped     float64 `json:"tps.gossiped"`
	TPSReceived     float64 `json:"tps.received"`
	TPSAccepted     float64 `json:"tps.accepted"`
	TPSDownloaded   float64 `json:"tps.downloaded"`
	BlocksFinalized float64 `json:"blocks.finalized"`

	// ms
	MaxQueryLatency  int64   `json:"query.latency.max.ms"`
	MinQueryLatency  int64   `json:"query.latency.min.ms"`
	MeanQueryLatency float64 `json:"query.latency.mean.ms"`
}

func (m *Metrics) MarshalEvent(ev *zerolog.Event) {
	ev.Int64("blocks.queried", m.BlocksQueried)
	ev.Int64("tx.gossiped", m.TxGossiped)
	ev.Int64("tx.received", m.TxReceived)
	ev.Int64("tx.accepted", m.TxAccepted)
	ev.Int64("tx.downloaded", m.TxDownloaded)
	ev.Float64("bps.queried", m.TPSGossiped)
	ev.Float64("tps.gossiped", m.TPSReceived)
	ev.Float64("tps.received", m.TPSReceived)
	ev.Float64("tps.accepted", m.TPSAccepted)
	ev.Float64("tps.downloaded", m.TPSDownloaded)
	ev.Float64("blocks.finalized", m.BlocksFinalized)
	ev.Int64("query.latency.max.ms", m.MaxQueryLatency)
	ev.Int64("query.latency.min.ms", m.MinQueryLatency)
	ev.Float64("query.latency.mean.ms", m.MeanQueryLatency)
	ev.Msg("Updated metrics.")
}

func (m *Metrics) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueBatch(v,
		"blocks.queried", &m.BlocksQueried,
		"tx.gossiped", &m.TxGossiped,
		"tx.received", &m.TxReceived,
		"tx.accepted", &m.TxAccepted,
		"tx.downloaded", &m.TxDownloaded,
		"bps.queried", &m.TPSGossiped,
		"tps.gossiped", &m.TPSReceived,
		"tps.received", &m.TPSReceived,
		"tps.accepted", &m.TPSAccepted,
		"tps.downloaded", &m.TPSDownloaded,
		"blocks.finalized", &m.BlocksFinalized,
		"query.latency.max.ms", &m.MaxQueryLatency,
		"query.latency.min.ms", &m.MinQueryLatency,
		"query.latency.mean.ms", &m.MeanQueryLatency,
	)
}

func NewMetrics(ctx context.Context) *MetricsService {
	var registry = metrics.NewRegistry()

	m := &MetricsService{
		registry: registry,
		context:  ctx,

		queried:         metrics.NewRegisteredMeter("blocks.queried", registry),
		gossipedTX:      metrics.NewRegisteredMeter("tx.gossiped", registry),
		receivedTX:      metrics.NewRegisteredMeter("tx.received", registry),
		acceptedTX:      metrics.NewRegisteredMeter("tx.accepted", registry),
		downloadedTX:    metrics.NewRegisteredMeter("tx.downloaded", registry),
		finalizedBlocks: metrics.NewRegisteredMeter("block.finalized", registry),
		queryLatency:    metrics.NewRegisteredTimer("query.latency", registry),
	}

	go m.start()

	return m
}

func (m *MetricsService) start() {
	m.ticker = time.NewTicker(time.Second)
	defer m.ticker.Stop()

	for {
		select {
		case t := <-m.ticker.C:
			m.LastPolled = t
			m.LastMetrics = &Metrics{
				BlocksQueried:    m.queried.Count(),
				TxGossiped:       m.gossipedTX.Count(),
				TxReceived:       m.receivedTX.Count(),
				TxAccepted:       m.acceptedTX.Count(),
				TxDownloaded:     m.downloadedTX.Count(),
				BPSQueried:       m.queried.RateMean(),
				TPSGossiped:      m.gossipedTX.RateMean(),
				TPSReceived:      m.receivedTX.RateMean(),
				TPSAccepted:      m.acceptedTX.RateMean(),
				TPSDownloaded:    m.downloadedTX.RateMean(),
				BlocksFinalized:  m.finalizedBlocks.RateMean(),
				MaxQueryLatency:  m.queryLatency.Max() / (1.0e+7),
				MinQueryLatency:  m.queryLatency.Min() / (1.0e+7),
				MeanQueryLatency: m.queryLatency.Mean() / (1.0e+7),
			}

			m.LastMetrics.MarshalEvent(log.Metrics().Info())

		case <-m.context.Done():
			return
		}
	}
}

func (m *MetricsService) Stop() {
	m.queried.Stop()
	m.gossipedTX.Stop()
	m.receivedTX.Stop()
	m.acceptedTX.Stop()
	m.downloadedTX.Stop()
	m.finalizedBlocks.Stop()
	m.queryLatency.Stop()
}
