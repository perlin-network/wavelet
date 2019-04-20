package wavelet

import (
	"github.com/perlin-network/wavelet/common"
	"sync"
)

const (
	SnowballDefaultK     = 1
	SnowballDefaultAlpha = float64(0.8)
	SnowballDefaultBeta  = 10
)

type Snowball struct {
	sync.RWMutex

	k, beta int
	alpha   float64

	preferredID, lastID common.TransactionID

	counts map[common.TransactionID]int
	count  int

	decided bool

	transactions map[common.TransactionID]Transaction
}

func NewSnowball() *Snowball {
	return &Snowball{
		k:     SnowballDefaultK,
		beta:  SnowballDefaultBeta,
		alpha: SnowballDefaultAlpha,

		counts:       make(map[common.TransactionID]int),
		transactions: make(map[common.TransactionID]Transaction),
	}
}

func (s *Snowball) WithK(k int) *Snowball {
	s.Lock()
	s.k = k
	s.Unlock()

	return s
}

func (s *Snowball) WithAlpha(alpha float64) *Snowball {
	s.Lock()
	s.alpha = alpha
	s.Unlock()

	return s
}

func (s *Snowball) WithBeta(beta int) *Snowball {
	s.Lock()
	s.beta = beta
	s.Unlock()

	return s
}

func (s *Snowball) Reset() {
	s.Lock()

	s.preferredID = common.ZeroTransactionID
	s.lastID = common.ZeroTransactionID

	s.counts = make(map[common.TransactionID]int)
	s.count = 0

	s.decided = false

	s.transactions = make(map[common.TransactionID]Transaction)

	s.Unlock()
}

func (s *Snowball) Tick(counts map[common.TransactionID]float64, transactions map[common.TransactionID]Transaction) {
	s.Lock()
	defer s.Unlock()

	if s.decided {
		return
	}

	for id, tx := range transactions {
		if _, exists := s.transactions[id]; !exists {
			s.transactions[id] = tx
		}
	}

	for preferredID, count := range counts {
		if count >= s.alpha {
			s.counts[preferredID]++

			if s.counts[preferredID] > s.counts[s.preferredID] {
				s.preferredID = preferredID
			}

			if s.lastID == common.ZeroTransactionID || s.lastID != preferredID {
				s.lastID = preferredID
				s.count = 0
			} else {
				s.count++

				if s.count > s.beta {
					s.decided = true
				}
			}

			break
		}
	}
}

func (s *Snowball) Prefer(tx Transaction) {
	s.Lock()

	if _, exists := s.transactions[tx.id]; !exists {
		s.transactions[tx.id] = tx
	}

	s.preferredID = tx.id

	s.Unlock()
}

func (s *Snowball) Preferred() *Transaction {
	s.RLock()
	defer s.RUnlock()

	if s.preferredID == common.ZeroTransactionID {
		return nil
	}

	if preferred, exists := s.transactions[s.preferredID]; exists {
		return &preferred
	}

	return nil
}

func (s *Snowball) Decided() bool {
	s.RLock()
	decided := s.decided
	s.RUnlock()

	return decided
}
