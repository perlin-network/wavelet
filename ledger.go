package wavelet

import (
	"github.com/lytics/hll"
	"github.com/perlin-network/graph/conflict"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/graph/system"
	"github.com/perlin-network/wavelet/log"
	"github.com/phf/go-queue/queue"
	"github.com/sasha-s/go-IBLT"
	"sort"
	"sync"
	"time"
)

var (
	BucketAccepted      = writeBytes("accepted_")
	BucketAcceptedIndex = writeBytes("i.accepted_")

	BucketAcceptPending = writeBytes("p.accepted_")

	KeyTransactionIBLT = writeBytes("tx_iblt")
)

type Ledger struct {
	state
	rpc

	*database.Store
	*graph.Graph
	*conflict.Resolver

	iblt      *iblt.Filter
	ibltMutex sync.Mutex

	kill chan struct{}
}

func NewLedger(databasePath, servicesPath string) *Ledger {
	store := database.New(databasePath)

	graph := graph.New(store)
	resolver := conflict.New(graph)

	ledger := &Ledger{
		Store:    store,
		Graph:    graph,
		Resolver: resolver,

		kill: make(chan struct{}),
	}

	ledger.state = state{Ledger: ledger}
	ledger.rpc = rpc{Ledger: ledger}

	if store.Size(BucketAccounts) == 0 {
		BIGBANG(ledger)
	}

	ledger.iblt = iblt.New(6, 4096)

	if encoded, err := store.Get(KeyTransactionIBLT); err == nil {
		err := ledger.iblt.UnmarshalBinary(encoded)

		if err != nil {
			log.Error().Err(err).Msg("Failed to decode transaction IBLT from database.")
		}
	}

	ledger.registerServicePath(servicesPath)

	graph.AddOnReceiveHandler(ledger.ensureSafeCommittable)

	return ledger
}

func (ledger *Ledger) WithIBLT(call func(*iblt.Filter) interface{}) interface{} {
	ledger.ibltMutex.Lock()
	value := call(ledger.iblt)
	ledger.ibltMutex.Unlock()

	return value
}

func (ledger *Ledger) Init() {
	go ledger.updateAcceptedTransactionsLoop()
}

// UpdateAcceptedTransactions incrementally from the root of the graph updates whether
// or not all transactions this node knows about are accepted.
//
// The graph will be incrementally checked for updates periodically. Ideally, you should
// execute this function in a new goroutine.
func (ledger *Ledger) updateAcceptedTransactionsLoop() {
	timer := time.NewTicker(100 * time.Millisecond)

	for {
		select {
		case <-ledger.kill:
			break
		case <-timer.C:
			ledger.updateAcceptedTransactions()
		}
	}

	timer.Stop()
}

// WasAccepted returns whether or not a transaction given by its symbol was stored to be accepted
// inside the database.
func (ledger *Ledger) WasAccepted(symbol string) bool {
	exists, _ := ledger.Has(merge(BucketAccepted, writeBytes(symbol)))
	return exists
}

// GetAcceptedByIndex gets an accepted transaction by its index.
func (ledger *Ledger) GetAcceptedByIndex(index uint64) (*database.Transaction, error) {
	symbolBytes, err := ledger.Get(merge(BucketAcceptedIndex, writeUint64(index)))
	if err != nil {
		return nil, err
	}

	return ledger.GetBySymbol(writeString(symbolBytes))
}

// QueueForAcceptance queues a transaction awaiting to be accepted.
func (ledger *Ledger) QueueForAcceptance(symbol string) error {
	return ledger.Put(merge(BucketAcceptPending, writeBytes(symbol)), []byte{0})
}

// updateAcceptedTransactions incrementally from the root of the graph updates whether
// or not all transactions this node knows about are accepted.
func (ledger *Ledger) updateAcceptedTransactions() {
	// If there are no accepted transactions and none are pending, add the very first transaction.
	if ledger.Size(BucketAcceptPending) == 0 && ledger.Size(BucketAcceptedIndex) == 0 {
		tx, err := ledger.GetByIndex(0)
		if err != nil {
			return
		}

		ledger.Put(merge(BucketAcceptPending, writeBytes(tx.Id)), []byte{0})
	}

	var acceptedList []string
	var pendingList []pending

	ledger.ForEachKey(BucketAcceptPending, func(k []byte) error {
		symbol := string(k)

		pendingList = append(pendingList)

		tx, err := ledger.GetBySymbol(symbol)
		if err != nil {
			return nil
		}

		depth, err := ledger.Store.GetDepthBySymbol(symbol)
		if err != nil {
			return nil
		}

		pendingList = append(pendingList, pending{tx, depth})

		return nil
	})

	sort.Slice(pendingList, func(i, j int) bool {
		if pendingList[i].depth < pendingList[j].depth {
			return true
		}

		if pendingList[i].depth > pendingList[j].depth {
			return false
		}

		return pendingList[i].tx.Id < pendingList[j].tx.Id
	})

	for _, pending := range pendingList {
		parentsAccepted := true
		for _, parent := range pending.tx.Parents {
			if !ledger.WasAccepted(parent) {
				parentsAccepted = false
				break
			}
		}

		if !parentsAccepted {
			continue
		}

		set, err := ledger.GetConflictSet(pending.tx.Sender, pending.tx.Nonce)
		if err != nil {
			continue
		}

		transactions := new(hll.Hll)
		err = transactions.UnmarshalPb(set.Transactions)

		if err != nil {
			continue
		}

		conflicting := !(transactions.Cardinality() == 1)

		if set.Count > system.Beta2 || (!conflicting && ledger.CountAscendants(pending.tx.Id, system.Beta1+1) > system.Beta1) {
			if !ledger.WasAccepted(pending.tx.Id) {
				ledger.acceptTransaction(pending.tx)
				acceptedList = append(acceptedList, pending.tx.Id)
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

// ensureAccepted gets called every single time the preferred transaction of a conflict set changes.
//
// It ensures that preferred transactions that were accepted, which should instead be rejected get
// reverted alongside all of their ascendant transactions.
func (ledger *Ledger) ensureAccepted(set *database.ConflictSet) error {
	transactions := new(hll.Hll)

	err := transactions.UnmarshalPb(set.Transactions)

	if err != nil {
		return err
	}

	// If the preferred transaction of a conflict set was accepted (due to safe early commit) and there are now transactions
	// conflicting with it, un-accept it.
	if conflicting := !(transactions.Cardinality() == 1); conflicting && ledger.WasAccepted(set.Preferred) && set.Count <= system.Beta2 {
		ledger.revertTransaction(set.Preferred, true)
	}

	return nil
}

// acceptTransaction accepts a transaction and ensures the transaction is not pending acceptance inside the graph.
// The children of said accepted transaction thereafter get queued to pending acceptance.
func (ledger *Ledger) acceptTransaction(tx *database.Transaction) {
	index, err := ledger.NextSequence(BucketAcceptedIndex)
	if err != nil {
		return
	}

	ledger.Put(merge(BucketAccepted, writeBytes(tx.Id)), writeUint64(index))
	ledger.Put(merge(BucketAcceptedIndex, writeUint64(index)), writeBytes(tx.Id))
	ledger.Delete(merge(BucketAcceptPending, writeBytes(tx.Id)))

	// If the transaction has accepted children, revert all of the transactions ascendants.
	if children, err := ledger.GetChildrenBySymbol(tx.Id); err == nil && len(children.Transactions) > 0 {
		for _, child := range children.Transactions {
			if ledger.WasAccepted(child) {
				ledger.revertTransaction(child, false)
			}
		}
	}

	// Apply transaction to the ledger state.
	err = ledger.applyTransaction(tx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to apply transaction.")
	}

	visited := make(map[string]struct{})

	queue := queue.New()
	queue.PushBack(tx.Id)

	for queue.Len() > 0 {
		popped := queue.PopFront().(string)

		children, err := ledger.GetChildrenBySymbol(popped)
		if err != nil {
			continue
		}

		for _, child := range children.Transactions {
			if _, seen := visited[child]; !seen {
				visited[child] = struct{}{}

				if !ledger.WasAccepted(child) {
					ledger.Put(merge(BucketAcceptPending, writeBytes(child)), []byte{0})
				} else if !ledger.WasApplied(child) {
					// If the child was already accepted but not yet applied, apply it to the ledger state.

					tx, err := ledger.GetBySymbol(child)
					if err != nil {
						continue
					}

					ledger.applyTransaction(tx)
				}
				queue.PushBack(child)

			}
		}
	}
}

// revertTransaction sets a transaction and all of its ascendants to not be accepted.
func (ledger *Ledger) revertTransaction(symbol string, revertAcceptance bool) {
	visited := make(map[string]struct{})

	queue := queue.New()
	queue.PushBack(symbol)

	var pendingList []pending

	for queue.Len() > 0 {
		popped := queue.PopFront().(string)

		tx, err := ledger.GetBySymbol(popped)
		if err != nil {
			continue
		}

		depth, err := ledger.GetDepthBySymbol(popped)
		if err != nil {
			continue
		}

		pendingList = append(pendingList, pending{tx, depth})

		indexBytes, err := ledger.Get(merge(BucketAccepted, writeBytes(popped)))
		if err != nil {
			continue
		}

		if revertAcceptance {
			ledger.Delete(merge(BucketAcceptedIndex, indexBytes))
			ledger.Delete(merge(BucketAccepted, writeBytes(popped)))

			ledger.Put(merge(BucketAcceptPending, writeBytes(popped)), []byte{0})
		}

		children, err := ledger.GetChildrenBySymbol(popped)
		if err != nil {
			continue
		}

		for _, child := range children.Transactions {
			if _, seen := visited[child]; !seen {
				visited[child] = struct{}{}

				if ledger.WasApplied(child) && ledger.WasAccepted(child) {
					queue.PushBack(child)
				}
			}
		}
	}

	// Sort in ascending lexicographically-least topological order.
	sort.Slice(pendingList, func(i, j int) bool {
		if pendingList[i].depth > pendingList[j].depth {
			return true
		}

		if pendingList[i].depth < pendingList[j].depth {
			return false
		}

		return pendingList[i].tx.Id > pendingList[j].tx.Id
	})

	// Revert list of transactions from ledger state.
	ledger.doRevertTransaction(&pendingList)

	log.Debug().Int("num_reverted", len(pendingList)).Msg("Reverted transactions.")
}

// ensureSafeCommittable ensures that incoming transactions which conflict with any
// of the transactions on our graph are not accepted.
func (ledger *Ledger) ensureSafeCommittable(index uint64, tx *database.Transaction) error {
	set, err := ledger.GetConflictSet(tx.Sender, tx.Nonce)

	if err != nil {
		return err
	}

	return ledger.ensureAccepted(set)
}
