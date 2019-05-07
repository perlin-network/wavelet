package wavelet

import (
	"context"
	"encoding/json"
	"github.com/perlin-network/wavelet/log"
	"github.com/rcrowley/go-metrics"
	"time"
)

type Metrics struct {
	registry metrics.Registry

	receivedTX metrics.Meter
	acceptedTX metrics.Meter
}

func NewMetrics() *Metrics {
	registry := metrics.NewRegistry()

	receivedTX := metrics.NewRegisteredMeter("tx.received", registry)
	acceptedTX := metrics.NewRegisteredMeter("tx.accepted", registry)

	return &Metrics{
		registry:   registry,
		receivedTX: receivedTX,
		acceptedTX: acceptedTX,
	}
}

func (m *Metrics) Stop() {
	m.acceptedTX.Stop()
	m.receivedTX.Stop()
}

func (m *Metrics) runLogger(ctx context.Context) {
	logger := log.Metrics()

	for {
		select {
		case <-time.After(5 * time.Second):
			if b, err := json.Marshal(m.registry); err == nil {
				logger.Log().RawJSON("metrics", b).Msg("Ledger metrics.")
			}
		case <-ctx.Done():
			return
		}
	}
}
