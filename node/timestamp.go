package node

import (
	"github.com/gogo/protobuf/sortkeys"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/params"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
)

// validateTimestamp validates the authenticity of the timestamp attached to a wired transaction.
func validateTimestamp(ledger *wavelet.LoopHandle, tx *wire.Transaction) error {
	var samples []int64

	q := queue.New()

	for _, parentID := range tx.Parents {
		q.PushBack(parentID)
	}

	count := 0

	for q.Len() > 0 && count < params.TimestampAncestorSampleSize {
		popped := q.PopFront().(string)

		var parent *database.Transaction
		var err error

		ledger.Do(func(ledger *wavelet.Ledger) {
			parent, err = ledger.Store.GetBySymbol(popped)
		})

		if err != nil {
			continue
		}

		samples = append(samples, parent.Timestamp)

		for _, p := range parent.Parents {
			q.PushBack(p)
		}

		count++
	}

	var median int64

	if len(samples) == 0 {
		return errors.New("no timestamp samples available")
	}

	sortkeys.Int64s(samples)

	if len(samples)%2 == 0 {
		median = (samples[len(samples)/2-1] + samples[len(samples)/2]) / 2
	} else {
		median = samples[len(samples)/2]
	}

	if tx.Timestamp <= median {
		return errors.New("timestamp is less than median timestamp")
	}

	return nil
}
