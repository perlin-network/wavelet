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
			ledger.processTransactions()
		}
	}

	timer.Stop()
}

func (ledger *Ledger) processTransactions() {
	threshold := ledger.Graph.Depth()

	start := threshold - system.Beta2
	if threshold < system.Beta2 {
		start = 0
	}

	accepted := make(map[uint64]map[string]bool)

	for depth := start; depth <= threshold; depth++ {
		ledger.Store.ForEachDepth(depth, func(index uint64, symbol string) error {
			status := ledger.IsAccepted(symbol)

			acceptedBytes, err := ledger.Store.Get(merge(BucketAccepted, writeBytes(symbol)))

			// Write the accepted status of transactions only if it ever changed.
			if accepted := readBoolean(acceptedBytes); err == nil && status == accepted {
				return nil
			}

			if accepted[depth] == nil {
				accepted[depth] = make(map[string]bool)
			}

			accepted[depth][symbol] = status
			return nil
		})
	}

	totalAccepted := 0

	// Save accepted transactions to the database.
	for _, symbols := range accepted {
		for symbol, accepted := range symbols {
			key := merge(BucketAccepted, writeBytes(symbol))

			err := ledger.Store.Put(key, writeBoolean(accepted))
			if err != nil {
				log.Error().Err(err).Msg("failed to write tx being accepted to db")
			}

			if accepted {
				totalAccepted++
			}
		}
	}

	if totalAccepted > 0 {
		log.Info().Msgf("Accepted %d transactions.", totalAccepted)
	}
}

func (ledger *Ledger) IsAccepted(symbol string) bool {
	tx, err := ledger.Store.GetBySymbol(symbol)
	if err != nil {
		return false
	}

	set, err := ledger.Resolver.GetConflictSet(tx.Sender, tx.Nonce)
	if err != nil {
		return false
	}

	// Condition 2 for being accepted. (beta 2 = 150)
	if set.Count > system.Beta2 {
		return true
	}

	visited := make(map[string]struct{})
	queue := tx.Parents

	// Check for Condition 1.
	for len(queue) > 0 {
		popped := queue[0]
		queue = queue[1:]

		tx, err = ledger.Store.GetBySymbol(popped)
		if err != nil {
			return false
		}

		set, err = ledger.Resolver.GetConflictSet(tx.Sender, tx.Nonce)
		if err != nil {
			return false
		}

		transactions := new(hll.Hll)
		err = transactions.UnmarshalPb(set.Transactions)

		// If we fail to unmarshal the HyperLogLog++ counter, then consider the transaction
		// not accepted to be safe. This implies the conflict set's bytes (in the database)
		// are corrupt.
		if err != nil {
			return false
		}

		conflicting := transactions.Cardinality() > 1

		// Condition 1 for being accepted. (beta 1 = 10)
		if conflicting || ledger.Graph.CountAscendants(symbol, system.Beta1+1) <= system.Beta1 {
			return false
		}

		acceptedBytes, err := ledger.Store.Get(merge(BucketAccepted, writeBytes(popped)))

		// If the transaction appears to have already been accepted, stop traversing through
		// its descendants.
		if accepted := readBoolean(acceptedBytes); err == nil && accepted {
			continue
		}

		for _, parent := range tx.Parents {
			if _, seen := visited[parent]; !seen {
				queue = append(queue, parent)
				visited[parent] = struct{}{}
			}
		}
	}

	return true
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
