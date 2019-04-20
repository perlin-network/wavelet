package wavelet

import (
	"github.com/perlin-network/wavelet/common"
	"golang.org/x/crypto/blake2b"
	"math"
	"sort"
)

// Nodes negotiate over which round to accept through Snowball. A round comprises of
// a Merkle root of the ledgers state proposed by the root transaction, alongside a
// single root transaction. Rounds are denoted by their ID, which is represented by
// BLAKE2b(merkle || root transactions content).
type Round struct {
	id     common.RoundID
	merkle common.MerkleNodeID
	root   Transaction
}

func NewRound(merkle common.MerkleNodeID, root Transaction) Round {
	return Round{
		id:     blake2b.Sum256(append(merkle[:], root.Marshal()...)),
		merkle: merkle,
		root:   root,
	}
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

// step atomically maintains the ledgers graph, and divides the graph from the bottom up into rounds.
//
// It maintains the ledgers Snowball instance while dividing up rounds, to see what the network believes
// is the preferred critical transaction selected to finalize the current ledgers round, and also be the
// root transaction for the next new round.
//
// It should be called repetitively as fast as possible in an infinite for loop, in a separate goroutine
// away from any other goroutines associated to the ledger.
func (l *Ledger) step() {
	// If we do not prefer any critical transaction yet, find a critical transaction to initially prefer first.

	if l.snowball.Preferred() == nil {
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

		if len(eligible) == 0 { // If there are no critical transactions for the round yet, discontinue.
			return
		}

		// Sort critical transactions by their depth, and pick the critical transaction
		// with the smallest depth as the nodes initial preferred transaction.
		//
		// The final selected critical transaction might change after a couple of
		// rounds with Snowball.

		sort.Slice(eligible, func(i, j int) bool {
			return eligible[i].Depth < eligible[j].Depth
		})

		// TODO(kenta): compute the merkle root of the transaction
		preferred := NewRound(common.ZeroMerkleNodeID, *eligible[0])
		l.snowball.Prefer(&preferred)
	}

	// TODO(kenta): query our peers, weigh their stakes, and find the response with the maximum
	// 	votes from our peers.

	var elected *Round

	l.snowball.Tick(elected)

	if l.snowball.Decided() {
		preferred := l.snowball.Preferred()
		root := l.graph.transactions[preferred.root.id]

		// TODO(kenta): apply preferred transaction to the ledger state, and check
		// 	if the new ledger state has the same merkle root as what is prescribed
		// 	in the preferred round.

		// TODO(kenta): clear out graph indices for transactions that are in the ancestry
		// 	of the preferred transaction.

		l.snowball.Reset()
		l.graph.reset(root)

		l.rounds[l.round] = *preferred
		l.round++

		// TODO(kenta): prune knowledge of rounds over time, say after 30 rounds and
		// 	also wipe away traces of their transactions.
	}
}
