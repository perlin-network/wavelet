package wavelet

import (
	"fmt"
	"github.com/dgryski/go-xxh3"
	"github.com/phf/go-queue/queue"
	"go.uber.org/atomic"
	"sync"
)

var (
	queuePool = sync.Pool{
		New: func() interface{} {
			return queue.New()
		},
	}
)

type mempoolShard struct {
	buffer sync.Map
	size   atomic.Uint32
}

type mempool struct {
	sync.RWMutex
	awaiting map[uint64]*mempoolShard

	queue chan []uint64
}

func newMempool() *mempool {
	return &mempool{
		awaiting: make(map[uint64]*mempoolShard),
		queue:    make(chan []uint64, 128),
	}
}

func (m *mempool) push(tx Transaction, missing []uint64) {
	awaiting := make([]uint64, 0, len(missing))

	for _, checksum := range missing {
		m.Lock()
		{
			if _, exists := m.awaiting[checksum]; !exists {
				m.awaiting[checksum] = new(mempoolShard)
				awaiting = append(awaiting, checksum)
			}

			shard := m.awaiting[checksum]

			if _, existed := shard.buffer.LoadOrStore(tx.Checksum, tx); !existed {
				shard.size.Inc()
			}
		}
		m.Unlock()
	}

	if len(awaiting) > 0 {
		m.queue <- awaiting
	}
}

func (m *mempool) mark(missing []uint64) {
	awaiting := make([]uint64, 0, len(missing))

	for _, checksum := range missing {
		m.Lock()
		{
			if _, exists := m.awaiting[checksum]; !exists {
				m.awaiting[checksum] = new(mempoolShard)
				awaiting = append(awaiting, checksum)
			}
		}
		m.Unlock()
	}

	if len(awaiting) > 0 {
		m.queue <- awaiting
	}
}

func (m *mempool) revisit(l *Ledger, checksum uint64) {
	shard, exists := m.loadShard(checksum)

	if !exists {
		return
	}

	unbuffered := make(map[uint64]Transaction)

	shard.buffer.Range(func(key, tx interface{}) bool {
		if err := l.addTransaction(tx.(Transaction)); err != nil {
			return true
		}

		unbuffered[tx.(Transaction).Checksum] = tx.(Transaction)
		return true
	})

	m.Lock()
	{
		for unbufferedChecksum, unbufferedTX := range unbuffered {
			// If a transaction T is unbuffered, make sure no other transactions we have yet
			// to receive is awaiting for the arrival of T.

			for _, parentID := range unbufferedTX.ParentIDs {
				checksum := xxh3.XXH3_64bits(parentID[:])

				shard, exists := m.awaiting[checksum]

				if !exists {
					continue
				}

				shard.buffer.Delete(unbufferedChecksum)
				shard.size.Dec()

				if shard.size.Load() == 0 {
					delete(m.awaiting, checksum)
				}
			}
		}

		delete(m.awaiting, checksum)

		if len(unbuffered) > 0 {
			fmt.Println("unbuffered", len(unbuffered), "transactions, and in total the buffer is of size", len(m.awaiting))
		}
	}
	m.Unlock()
}

func (m *mempool) clearOutdated(currentViewID uint64) {
	m.Lock()
	{
		for checksum, shard := range m.awaiting {
			shard.buffer.Range(func(key, tx interface{}) bool {
				if tx.(Transaction).ViewID < currentViewID {
					shard.buffer.Delete(tx.(Transaction).Checksum)
					shard.size.Dec()
				}

				return true
			})

			if shard.size.Load() == 0 {
				delete(m.awaiting, checksum)
			}
		}
	}
	m.Unlock()

}

func (m *mempool) loadShard(checksum uint64) (*mempoolShard, bool) {
	m.RLock()
	shard, exists := m.awaiting[checksum]
	m.RUnlock()

	return shard, exists
}
