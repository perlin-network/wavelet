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
	"github.com/phf/go-queue/queue"
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

	ledger := &Ledger{Store: store, Graph: graph, Resolver: resolver, pendingAcceptance: make(map[string]struct{})}

	graph.AddOnReceiveHandler(ledger.onReceiveTransaction)

	return ledger
}

// UpdateAcceptedTransactions incrementally from the root of the graph updates whether
// or not all transactions this node knows about are accepted.
//
// The graph will be incrementally checked for updates periodically. Ideally, you should
// execute this function in a new goroutine.
func (ledger *Ledger) UpdateAcceptedTransactions() {
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

// updateAcceptedTransactions incrementally from the root of the graph updates whether
// or not all transactions this node knows about are accepted.
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

	for symbol := range ledger.pendingAcceptance {
		tx, err := ledger.Store.GetBySymbol(symbol)
		if err != nil {
			continue
		}

		set, err := ledger.Resolver.GetConflictSet(tx.Sender, tx.Nonce)
		if err != nil {
			continue
		}

		transactions := new(hll.Hll)
		err = transactions.UnmarshalPb(set.Transactions)

		if err != nil {
			continue
		}

		conflicting := !(transactions.Cardinality() == 1)

		if set.Count > system.Beta2 || (!conflicting && ledger.Graph.CountAscendants(symbol, system.Beta1+1) > system.Beta1) {
			if !ledger.WasAccepted(symbol) {
				ledger.acceptTransaction(symbol)
				acceptedList = append(acceptedList, symbol)
			}
		}
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

// HandleSuccessfulQuery updates the conflict sets and acceptance of all transactions
// preceding a successfully queried transactions.
func (ledger *Ledger) HandleSuccessfulQuery(tx *database.Transaction) error {
	visited := make(map[string]struct{})

	queue := queue.New()
	queue.PushBack(tx.Id)

	for queue.Len() > 0 {
		popped := queue.PopFront().(string)

		tx, err := ledger.Store.GetBySymbol(popped)
		if err != nil {
			continue
		}

		set, err := ledger.Resolver.GetConflictSet(tx.Sender, tx.Nonce)
		if err != nil {
			continue
		}

		score, preferredScore := ledger.Graph.CountAscendants(popped, system.Beta2), ledger.Graph.CountAscendants(set.Preferred, system.Beta2)

		if score > preferredScore {
			ledger.ensureAccepted(set, set.Preferred)

			set.Preferred = popped
		}

		if popped != set.Last {
			set.Last = popped
			set.Count = 0
		} else {
			set.Count++
		}

		err = ledger.Resolver.SaveConflictSet(tx.Sender, tx.Nonce, set)
		if err != nil {
			continue
		}

		for _, parent := range tx.Parents {
			if _, seen := visited[parent]; !seen {
				visited[parent] = struct{}{}

				queue.PushBack(parent)
			}
		}
	}

	return nil
}

// ensureAccepted gets called every single time the preferred transaction of a conflict set changes.
//
// It ensures that preferred transactions that were accepted, which should instead be rejected get
// reverted alongside all of their ascendant transactions.
func (ledger *Ledger) ensureAccepted(set *database.ConflictSet, symbol string) error {
	if ledger.WasAccepted(symbol) {
		transactions := new(hll.Hll)

		err := transactions.UnmarshalPb(set.Transactions)

		if err != nil {
			return err
		}

		conflicting := !(transactions.Cardinality() == 1)

		// Check if the transaction is still accepted.
		stillAccepted := set.Count > system.Beta2 || (!conflicting && ledger.Graph.CountAscendants(symbol, system.Beta1+1) > system.Beta2)

		if !stillAccepted {
			ledger.revertTransaction(symbol)
		}
	}

	return nil
}

// WasAccepted returns whether or not a transaction given by its symbol
// was stored to be accepted inside the database.
func (ledger *Ledger) WasAccepted(symbol string) bool {
	bytes, _ := ledger.Store.Get(merge(BucketAccepted, writeBytes(symbol)))
	return readBoolean(bytes)
}

// acceptTransaction accepts a transaction and ensures the transaction is not pending acceptance inside the graph.
// The children of said accepted transaction thereafter get queued to pending acceptance.
//
// Should the transaction previously not have been accepted, the
func (ledger *Ledger) acceptTransaction(symbol string) {
	ledger.Store.Put(merge(BucketAccepted, writeBytes(symbol)), writeBoolean(true))

	delete(ledger.pendingAcceptance, symbol)

	visited := make(map[string]struct{})

	queue := queue.New()
	queue.PushBack(symbol)

	for queue.Len() > 0 {
		popped := queue.PopFront().(string)

		children, err := ledger.Store.GetChildrenBySymbol(popped)
		if err != nil {
			continue
		}

		for _, child := range children.Transactions {
			if _, seen := visited[child]; !seen {
				visited[child] = struct{}{}

				if _, pending := ledger.pendingAcceptance[child]; !pending {
					ledger.pendingAcceptance[child] = struct{}{}
				}

				queue.PushBack(child)

			}
		}
	}
}

// revertTransaction sets a transaction and all of its ascendants to not be accepted.
func (ledger *Ledger) revertTransaction(symbol string) {
	numReverted := 0

	visited := make(map[string]struct{})

	queue := queue.New()
	queue.PushBack(symbol)

	for queue.Len() > 0 {
		popped := queue.PopFront().(string)
		numReverted++

		ledger.Store.Delete(merge(BucketAccepted, writeBytes(popped)))

		children, err := ledger.Store.GetChildrenBySymbol(popped)
		if err != nil {
			continue
		}

		for _, child := range children.Transactions {
			if _, seen := visited[child]; !seen {
				visited[child] = struct{}{}

				queue.PushBack(child)
			}
		}
	}

	log.Debug().Int("num_reverted", numReverted).Msg("Reverted transactions.")
}

// onReceiveTransaction ensures that incoming transactions which conflict with any
// of the transactions on our graph are not accepted.
func (ledger *Ledger) onReceiveTransaction(index uint64, tx *database.Transaction) error {
	set, err := ledger.Resolver.GetConflictSet(tx.Sender, tx.Nonce)

	if err != nil {
		return err
	}

	return ledger.ensureAccepted(set, set.Preferred)
}
