package wavelet

import (
	"github.com/perlin-network/wavelet/avl"
	"math"
)

// Nodes negotiate over which round to accept through Snowball. A round comprises
// of a Merkle root of the ledgers state, and a root transaction. Rounds are
// represented by BLAKE2b(merkle || root transactions content).
type Round struct {
	merkle [avl.MerkleHashSize]byte
	root   Transaction
}

type Ledger struct {
	graph *Graph

	snowball *Snowball

	rounds map[uint64]Round
	round  uint64
}

func NewLedger() *Ledger {
	graph := NewGraph()

	return &Ledger{
		graph: graph,

		snowball: NewSnowball(),

		rounds: make(map[uint64]Round),
		round:  0,
	}
}

const MinDifficulty = 8

func (l *Ledger) step() {
	difficulty := l.graph.transactions[l.graph.rootID].ExpectedDifficulty(MinDifficulty)

	var eligible []*Transaction // Find all critical transactions for the current round.

	for i := difficulty; i < math.MaxUint8; i++ {
		candidates, exists := l.graph.seedIndex[difficulty]

		if !exists {
			continue
		}

		for candidateID := range candidates {
			eligible = append(eligible, l.graph.transactions[candidateID])
		}
	}

	if len(eligible) == 0 {
		return
	}
}
