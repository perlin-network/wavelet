package wavelet

import (
	"fmt"
	"github.com/dgryski/go-xxh3"
	"github.com/phf/go-queue/queue"
	"sync"
)

var (
	queuePool = sync.Pool{
		New: func() interface{} {
			return queue.New()
		},
	}
)

type mempool struct {
	sync.RWMutex

	awaiting map[uint64]map[uint64]Transaction
	queue    chan []uint64
}

func newMempool() *mempool {
	return &mempool{
		awaiting: make(map[uint64]map[uint64]Transaction),
		queue:    make(chan []uint64, 128),
	}
}

func (m *mempool) push(tx Transaction, missing []uint64) {
	awaiting := make([]uint64, 0, len(missing))

	for _, checksum := range missing {
		m.Lock()
		{
			if _, exists := m.awaiting[checksum]; !exists {
				m.awaiting[checksum] = make(map[uint64]Transaction)
				awaiting = append(awaiting, checksum)
			}

			m.awaiting[checksum][tx.Checksum] = tx
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
				m.awaiting[checksum] = make(map[uint64]Transaction)
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

	for _, tx := range shard {
		if err := l.addTransaction(tx); err != nil {
			continue
		}

		unbuffered[tx.Checksum] = tx
	}

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

				delete(shard, unbufferedChecksum)

				if len(shard) == 0 {
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
			for _, tx := range shard {
				if tx.ViewID < currentViewID {
					delete(shard, tx.Checksum)
				}
			}

			if len(shard) == 0 {
				delete(m.awaiting, checksum)
			}
		}
	}
	m.Unlock()

}

func (m *mempool) loadShard(checksum uint64) (map[uint64]Transaction, bool) {
	m.RLock()
	shard, exists := m.awaiting[checksum]
	m.RUnlock()

	return shard, exists
}
