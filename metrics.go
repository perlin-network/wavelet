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
