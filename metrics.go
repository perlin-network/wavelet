package wavelet

import (
	"github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/exp"
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

	//go metrics.Log(registry, 5*time.Second, log.New(os.Stderr, "metrics: ", log.Lmicroseconds))

	exp.Exp(registry)

	return &Metrics{
		registry:   registry,
		receivedTX: receivedTX,
		acceptedTX: acceptedTX,
	}
}
