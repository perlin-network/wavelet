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

	pendingAcceptance map[string]struct{}

	kill chan struct{}
}

func NewLedger() *Ledger {
	store := database.New("testdb")

	graph := graph.New(store)
	resolver := conflict.New(graph)

	return &Ledger{Store: store, Graph: graph, Resolver: resolver, pendingAcceptance: make(map[string]struct{})}
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
	// If there are no accepted transactions and none are pending, add the very first transaction.
	if len(ledger.pendingAcceptance) == 0 && ledger.Store.Size(BucketAccepted) == 0 {
		tx, err := ledger.Store.GetByIndex(0)
		if err != nil {
			return
		}

		ledger.pendingAcceptance[tx.Id] = struct{}{}
	}

	var acceptedList []string

	accept := func(symbol string, accepted bool) error {
		bytes, _ := ledger.Store.Get(merge(BucketAccepted, writeBytes(symbol)))
		wasAccepted := readBoolean(bytes)

		if !wasAccepted && accepted {
			ledger.Store.Put(merge(BucketAccepted, writeBytes(symbol)), writeBoolean(true))

			delete(ledger.pendingAcceptance, symbol)

			// Get the ascendants of the accepted transaction and have them pend being accepted.

			queue := []string{symbol}
			visited := make(map[string]struct{})

			for len(queue) > 0 {
				popped := queue[0]
				queue = queue[1:]

				ledger.Store.ForEachChild(popped, func(child string) error {
					if _, seen := visited[child]; !seen {
						ledger.pendingAcceptance[child] = struct{}{}

						queue = append(queue, child)
						visited[child] = struct{}{}
					}
					return nil
				})
			}

			acceptedList = append(acceptedList, symbol)
		}

		return nil
	}

	for symbol := range ledger.pendingAcceptance {
		tx, err := ledger.Store.GetBySymbol(symbol)
		if err != nil {
			accept(symbol, false)
			continue
		}

		set, err := ledger.Resolver.GetConflictSet(tx.Sender, tx.Nonce)
		if err != nil {
			accept(symbol, false)
			continue
		}

		transactions := new(hll.Hll)
		err = transactions.UnmarshalPb(set.Transactions)

		if err != nil {
			accept(symbol, false)
			continue
		}

		// Condition 2 for being accepted.
		if set.Count > system.Beta2 {
			accept(symbol, true)
			continue
		}

		// Condition 1 for being accepted.
		conflicting := transactions.Cardinality() > 1

		if !conflicting && ledger.Graph.CountAscendants(symbol, system.Beta1+1) > system.Beta1 {
			accept(symbol, true)
			continue
		}

		accept(symbol, false)
	}

	if len(acceptedList) > 0 {
		// Trim transaction IDs.
		for i := 0; i < len(acceptedList); i++ {
			acceptedList[i] = acceptedList[i][:10]
		}

		log.Info().Interface("accepted", acceptedList).Msgf("Accepted %d transactions.", len(acceptedList))
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

//func (ledger *Ledger) HandleQueryResponse(tx *database.Transaction) ()

func (ledger *Ledger) FindEligibleParents() ([]string, error) {
	return ledger.Resolver.FindEligibleParents()
}

func (ledger *Ledger) Cleanup() error {
	return ledger.Graph.Cleanup()
}
