package wavelet

import (
	"github.com/lytics/hll"
	"github.com/perlin-network/graph/conflict"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/graph/system"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/security"
	"github.com/pkg/errors"
	"time"
)

var (
	BucketAccepted = writeBytes("accepted_")
)

type Ledger struct {
	Store    *database.Store
	Graph    *graph.Graph
	Resolver *conflict.Resolver

	kill chan struct{}
}

func NewLedger() *Ledger {
	store := database.New("testdb")

	graph := graph.New(store)
	resolver := conflict.New(graph)

	return &Ledger{Store: store, Graph: graph, Resolver: resolver}
}

// ProcessTransactions starts a goroutine to periodically update the present
// ledger state given all incoming transactions.
//
// ProcessTransactions will look at all tr
func (ledger *Ledger) ProcessTransactions() {
	timer := time.NewTicker(1 * time.Second)

	for {
		select {
		case <-ledger.kill:
			break
		case <-timer.C:
			ledger.updatedAcceptedTransactions()
		}
	}

	timer.Stop()
}

func (ledger *Ledger) updatedAcceptedTransactions() {
	frontierDepth := ledger.Graph.Depth()

	numAccepted, numReverted := 0, 0

	for depth := uint64(0); depth <= frontierDepth; depth++ {
		ledger.Store.ForEachDepth(depth, func(index uint64, symbol string) error {
			bytes, _ := ledger.Store.Get(merge(BucketAccepted, writeBytes(symbol)))
			wasAccepted := readBoolean(bytes)

			accept := func(symbol string, accepted bool) error {
				// Revert transaction and all of its ascendants.
				if wasAccepted && !accepted {
					queue := []string{symbol}
					visited := map[string]struct{}{symbol: {}}

					for len(queue) > 0 {
						popped := queue[0]
						queue = queue[1:]

						numReverted++

						ledger.Store.Put(merge(BucketAccepted, writeBytes(popped)), writeBoolean(false))

						ledger.Store.ForEachChild(symbol, func(child string) error {
							queue = append(queue, child)
							visited[child] = struct{}{}

							return nil
						})
					}
				}

				if !wasAccepted && accepted {
					ledger.Store.Put(merge(BucketAccepted, writeBytes(symbol)), writeBoolean(true))
					numAccepted++
				}

				return nil
			}

			tx, err := ledger.Store.GetBySymbol(symbol)
			if err != nil {
				return accept(symbol, false)
			}

			set, err := ledger.Resolver.GetConflictSet(tx.Sender, tx.Nonce)
			if err != nil {
				return accept(symbol, false)
			}

			transactions := new(hll.Hll)
			err = transactions.UnmarshalPb(set.Transactions)

			if err != nil {
				return accept(symbol, false)
			}

			// Condition 2 for being accepted.
			if set.Count > system.Beta2 {
				return accept(symbol, true)
			}

			// Condition 1 for being accepted.
			conflicting := transactions.Cardinality() > 1

			if !conflicting && ledger.Graph.CountAscendants(symbol, system.Beta1+1) > system.Beta1 {
				return accept(symbol, true)
			}

			return accept(symbol, false)
		})
	}

	if numAccepted > 0 {
		log.Info().Msgf("Accepted %d transactions.", numAccepted)
	}

	if numReverted > 0 {
		log.Info().Msgf("Reverted %d transactions.", numReverted)
	}
}

// RespondToQuery provides a response should we be selected as one of the K peers
// that votes whether or not we personally strongly prefer a transaction which we
// receive over the wire.
//
// Our response is `true` should we strongly prefer a transaction, or `false` otherwise.
func (ledger *Ledger) RespondToQuery(wired *wire.Transaction) (string, bool, error) {
	if validated, err := security.ValidateWiredTransaction(wired); err != nil || !validated {
		return "", false, errors.Wrap(err, "failed to validate incoming tx")
	}

	id, err := ledger.Graph.Receive(wired)

	if err != nil {
		return "", false, errors.Wrap(err, "failed to add incoming tx to graph")
	}

	return id, ledger.Resolver.IsStronglyPreferred(id), nil
}

func (ledger *Ledger) FindEligibleParents() ([]string, error) {
	return ledger.Resolver.FindEligibleParents()
}

func (ledger *Ledger) Cleanup() error {
	return ledger.Graph.Cleanup()
}
