package wavelet

import (
	"bytes"
	"encoding/hex"
	"github.com/lytics/hll"
	"github.com/perlin-network/graph/conflict"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/graph/system"
	"github.com/perlin-network/wavelet/events"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/params"
	"github.com/perlin-network/wavelet/stats"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"sort"
	"time"
)

var (
	BucketAccepted      = writeBytes("accepted_")
	BucketAcceptedIndex = writeBytes("i.accepted_")

	BucketAcceptPending = writeBytes("p.accepted_")

	KeyGenesisApplied = writeBytes("@genesis_applied")
)

var (
	ErrStop = errors.New("stop")
)

var _ LedgerInterface = (*Ledger)(nil)

type Ledger struct {
	state
	rpc

	*database.Store
	*graph.Graph
	*conflict.Resolver

	kill chan struct{}

	stepping               bool
	lastUpdateAcceptedTime time.Time

	accounts *Accounts
}

func NewLedger(databasePath, genesisPath string) *Ledger {
	store := database.New(databasePath)

	log.Info().Str("db_path", databasePath).Msg("Database has been loaded.")

	graph := graph.New(store)
	resolver := conflict.New(graph)

	ledger := &Ledger{
		Store:    store,
		Graph:    graph,
		Resolver: resolver,

		kill: make(chan struct{}),
	}

	ledger.accounts = newAccounts(*store)

	ledger.state = state{Ledger: ledger}
	ledger.rpc = rpc{Ledger: ledger}

	if len(genesisPath) > 0 {
		if x, _ := ledger.Store.Get(KeyGenesisApplied); x == nil {
			genesis, err := ReadGenesis(ledger, genesisPath)

			if err != nil {
				log.Error().Err(err).Msgf("Could not read genesis details which were expected to be at: %s", genesisPath)
			}

			for _, account := range genesis {
				ledger.accounts.save(account)
			}

			log.Info().Str("file", genesisPath).Int("num_accounts", len(genesis)).Msg("Successfully seeded the genesis of this node.")
			ledger.Store.Put(KeyGenesisApplied, []byte("true"))
		}
	}

	graph.AddOnReceiveHandler(ledger.ensureSafeCommittable)

	return ledger
}

// Accounts returns the accounts storage
func (ledger *Ledger) Accounts() *Accounts {
	return ledger.accounts
}

// Step will perform one single time step of all periodic tasks within the ledger.
func (ledger *Ledger) Step(force bool) {
	if ledger.stepping {
		return
	}

	ledger.stepping = true

	current := time.Now()

	if force || current.Sub(ledger.lastUpdateAcceptedTime) >= time.Duration(params.GraphUpdatePeriodMs)*time.Millisecond {
		ledger.updateAcceptedTransactions()
		ledger.lastUpdateAcceptedTime = current
	}

	ledger.stepping = false
}

// WasAccepted returns whether or not a transaction given by its symbol was stored to be accepted
// inside the database.
func (ledger *Ledger) WasAccepted(symbol []byte) bool {
	exists, _ := ledger.Has(merge(BucketAccepted, symbol))
	return exists
}

// GetAcceptedByIndex gets an accepted transaction by its index.
func (ledger *Ledger) GetAcceptedByIndex(index uint64) (*database.Transaction, error) {
	symbolBytes, err := ledger.Get(merge(BucketAcceptedIndex, writeUint64(index)))
	if err != nil {
		return nil, err
	}

	return ledger.GetBySymbol(symbolBytes)
}

// QueueForAcceptance queues a transaction awaiting to be accepted.
func (ledger *Ledger) QueueForAcceptance(symbol []byte) error {
	return ledger.Put(merge(BucketAcceptPending, symbol), []byte{0})
}

// UpdateAcceptedTransactions incrementally from the root of the graph updates whether
// or not all transactions this node knows about are accepted.
func (ledger *Ledger) updateAcceptedTransactions() {
	// If there are no accepted transactions and none are pending, add the very first transaction.
	if ledger.Size(BucketAcceptPending) == 0 && ledger.NumAcceptedTransactions() == 0 {
		var tx *database.Transaction

		err := ledger.ForEachDepth(0, func(symbol []byte) error {
			first, err := ledger.GetBySymbol(symbol)
			if err != nil {
				return err
			}

			tx = first
			return ErrStop
		})

		if err != ErrStop {
			return
		}

		err = ledger.QueueForAcceptance(tx.Id)

		if err != nil {
			return
		}
	}

	var acceptedList [][]byte
	var pendingList []pending

	ledger.ForEachKey(BucketAcceptPending, func(symbol []byte) error {

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

		return bytes.Compare(pendingList[i].tx.Id, pendingList[j].tx.Id) == -1
	})

	stats.SetNumPendingTx(int64(len(pendingList)))

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

		if (bytes.Equal(set.Preferred, pending.tx.Id) && set.Count > system.Beta2) || (!conflicting && ledger.CountAscendants(pending.tx.Id, system.Beta1+1) > system.Beta1) {
			if !ledger.WasAccepted(pending.tx.Id) {
				ledger.acceptTransaction(pending.tx)
				acceptedList = append(acceptedList, pending.tx.Id)
			}
		}
	}

	if len(acceptedList) > 0 {
		var acceptedListStr = make([]string, len(acceptedList))

		// Trim and encode transaction IDs.
		for i := 0; i < len(acceptedList); i++ {
			acceptedListStr[i] = hex.EncodeToString(acceptedList[i][:10])
		}

		log.Debug().Interface("accepted", acceptedListStr).Msgf("Accepted %d transactions.", len(acceptedListStr))
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
		//ledger.revertTransaction(set.Preferred, true)
		log.Warn().Msg("safe early commit conflicts with new transactions")
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

	ledger.Put(merge(BucketAccepted, tx.Id), writeUint64(index))
	ledger.Put(merge(BucketAcceptedIndex, writeUint64(index)), tx.Id)
	ledger.Delete(merge(BucketAcceptPending, tx.Id))

	stats.IncAcceptedTransactions(tx.Tag)
	go events.Publish(nil, &events.TransactionAcceptedEvent{ID: tx.Id})

	depth, err := ledger.Store.GetDepthBySymbol(tx.Id)
	if err != nil {
		log.Error().Err(err).Msg("cannot get depth")
		return
	}

	var refTxId []byte
	var gotRefTxId bool
	var pendingReapply [][]byte

	ledger.Store.ForEachDepth(depth, func(symbol []byte) error {
		if bytes.Compare(symbol, tx.Id) > 0 {
			if ledger.WasAccepted(symbol) {
				if !gotRefTxId {
					refTxId = symbol
					gotRefTxId = true
				}
				pendingReapply = append(pendingReapply, symbol)
			}
		}
		return nil
	})

	for i := depth + 1; ; i++ {
		gotAnyTx := false
		ledger.Store.ForEachDepth(i, func(symbol []byte) error {
			gotAnyTx = true
			if ledger.WasAccepted(symbol) {
				if !gotRefTxId {
					refTxId = symbol
					gotRefTxId = true
				}
				pendingReapply = append(pendingReapply, symbol)
			}
			return nil
		})
		if !gotAnyTx {
			break
		}
	}

	if gotRefTxId {
		ledger.revertTransaction(refTxId)
	}

	err = ledger.applyTransaction(tx)
	if err != nil {
		log.Warn().Err(err).Str("symbol", hex.EncodeToString(tx.Id)).Msg("failed to apply transaction")
	}

	for _, symbol := range pendingReapply {
		reTx, err := ledger.GetBySymbol(symbol)
		if err != nil {
			continue
		}
		err = ledger.applyTransaction(reTx)
		if err != nil {
			log.Warn().Err(err).Str("symbol", hex.EncodeToString(symbol)).Msg("failed to reapply transaction")
		}
	}

	visited := make(map[string]struct{})

	queue := queue.New()
	queue.PushBack(tx.Id)

	for queue.Len() > 0 {
		popped := queue.PopFront().([]byte)

		children, err := ledger.GetChildrenBySymbol(popped)
		if err != nil {
			continue
		}

		for _, child := range children.Transactions {
			if _, seen := visited[writeString(child)]; !seen {
				visited[writeString(child)] = struct{}{}

				if !ledger.WasAccepted(child) {
					ledger.QueueForAcceptance(child)
				}
				queue.PushBack(child)

			}
		}
	}
}

// revertTransaction sets a transaction and all of its ascendants to not be accepted.
func (ledger *Ledger) revertTransaction(symbol []byte) {
	tx, err := ledger.GetBySymbol(symbol)
	if err != nil {
		log.Error().Err(err).Msg("cannot get transaction for reverting")
		return
	}

	stateRoot, err := ledger.Store.Get(merge(BucketPreStates, []byte(tx.Id)))
	if err != nil {
		log.Error().Err(err).Msg("cannot get state root for reverting")
		return
	}

	ledger.Accounts().SetRoot(stateRoot)

	log.Debug().Str("key", hex.EncodeToString(symbol)).Str("state_root", hex.EncodeToString(stateRoot)).Msg("Reverted transaction.")
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
