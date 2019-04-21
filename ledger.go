package wavelet

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"math"
	"sort"
	"sync"
	"time"
)

type Ledger struct {
	ctx    context.Context
	cancel context.CancelFunc

	keys *skademlia.Keypair

	accounts *accounts
	graph    *Graph

	snowball *Snowball
	resolver *Snowball

	processors map[byte]TransactionProcessor

	rounds map[uint64]Round
	round  uint64

	mu sync.RWMutex

	BroadcastQueue chan<- EventBroadcast
	broadcastQueue <-chan EventBroadcast

	GossipIn chan<- EventIncomingGossip
	gossipIn <-chan EventIncomingGossip

	GossipOut <-chan EventGossip
	gossipOut chan<- EventGossip

	QueryIn chan<- EventIncomingQuery
	queryIn <-chan EventIncomingQuery

	QueryOut <-chan EventQuery
	queryOut chan<- EventQuery

	OutOfSyncIn chan<- EventIncomingOutOfSyncCheck
	outOfSyncIn <-chan EventIncomingOutOfSyncCheck

	OutOfSyncOut <-chan EventOutOfSyncCheck
	outOfSyncOut chan<- EventOutOfSyncCheck

	SyncInitIn chan<- EventIncomingSyncInit
	syncInitIn <-chan EventIncomingSyncInit

	SyncInitOut <-chan EventSyncInit
	syncInitOut chan<- EventSyncInit

	SyncDiffIn chan<- EventIncomingSyncDiff
	syncDiffIn <-chan EventIncomingSyncDiff

	SyncDiffOut <-chan EventSyncDiff
	syncDiffOut chan<- EventSyncDiff

	SyncTxIn chan<- EventIncomingSyncTX
	syncTxIn <-chan EventIncomingSyncTX

	SyncTxOut <-chan EventSyncTX
	syncTxOut chan<- EventSyncTX

	LatestViewIn chan<- EventIncomingLatestView
	latestViewIn <-chan EventIncomingLatestView

	LatestViewOut <-chan EventLatestView
	latestViewOut chan<- EventLatestView
}

func NewLedger(keys *skademlia.Keypair) *Ledger {
	ctx, cancel := context.WithCancel(context.Background())

	broadcastQueue := make(chan EventBroadcast, 1024)

	gossipIn := make(chan EventIncomingGossip, 128)
	gossipOut := make(chan EventGossip, 128)

	queryIn := make(chan EventIncomingQuery, 128)
	queryOut := make(chan EventQuery, 128)

	outOfSyncIn := make(chan EventIncomingOutOfSyncCheck, 16)
	outOfSyncOut := make(chan EventOutOfSyncCheck, 16)

	syncInitIn := make(chan EventIncomingSyncInit, 16)
	syncInitOut := make(chan EventSyncInit, 16)

	syncDiffIn := make(chan EventIncomingSyncDiff, 128)
	syncDiffOut := make(chan EventSyncDiff, 128)

	syncTxIn := make(chan EventIncomingSyncTX, 16)
	syncTxOut := make(chan EventSyncTX, 16)

	latestViewIn := make(chan EventIncomingLatestView, 16)
	latestViewOut := make(chan EventLatestView, 16)

	accounts := newAccounts(store.NewInmem())
	round := performInception(accounts.tree, nil)

	if err := accounts.commit(nil); err != nil {
		panic(err)
	}

	graph := NewGraph(&round)

	return &Ledger{
		ctx:    ctx,
		cancel: cancel,

		keys: keys,

		accounts: accounts,
		graph:    graph,

		snowball: NewSnowball().WithK(sys.SnowballQueryK).WithAlpha(sys.SnowballQueryAlpha).WithBeta(sys.SnowballQueryBeta),
		resolver: NewSnowball().WithK(sys.SnowballSyncK).WithAlpha(sys.SnowballSyncAlpha).WithBeta(sys.SnowballSyncBeta),

		processors: map[byte]TransactionProcessor{
			sys.TagNop:      ProcessNopTransaction,
			sys.TagTransfer: ProcessTransferTransaction,
			sys.TagContract: ProcessContractTransaction,
			sys.TagStake:    ProcessStakeTransaction,
		},

		rounds: map[uint64]Round{round.Index: round},
		round:  1,

		BroadcastQueue: broadcastQueue,
		broadcastQueue: broadcastQueue,

		GossipIn: gossipIn,
		gossipIn: gossipIn,

		GossipOut: gossipOut,
		gossipOut: gossipOut,

		QueryIn: queryIn,
		queryIn: queryIn,

		QueryOut: queryOut,
		queryOut: queryOut,

		OutOfSyncIn: outOfSyncIn,
		outOfSyncIn: outOfSyncIn,

		OutOfSyncOut: outOfSyncOut,
		outOfSyncOut: outOfSyncOut,

		SyncInitIn: syncInitIn,
		syncInitIn: syncInitIn,

		SyncInitOut: syncInitOut,
		syncInitOut: syncInitOut,

		SyncDiffIn: syncDiffIn,
		syncDiffIn: syncDiffIn,

		SyncDiffOut: syncDiffOut,
		syncDiffOut: syncDiffOut,

		SyncTxIn: syncTxIn,
		syncTxIn: syncTxIn,

		SyncTxOut: syncTxOut,
		syncTxOut: syncTxOut,

		LatestViewIn: latestViewIn,
		latestViewIn: latestViewIn,

		LatestViewOut: latestViewOut,
		latestViewOut: latestViewOut,
	}
}

func NewTransaction(creator *skademlia.Keypair, tag byte, payload []byte) Transaction {
	tx := Transaction{Tag: tag, Payload: payload}

	var buf [8]byte
	// TODO(kenta): nonce

	tx.Creator = creator.PublicKey()
	tx.CreatorSignature = edwards25519.Sign(creator.PrivateKey(), append(buf[:], append([]byte{tx.Tag}, tx.Payload...)...))

	return tx
}

func (l *Ledger) attachSenderToTransaction(tx Transaction) (Transaction, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	tx.Sender = l.keys.PublicKey()
	tx.ParentIDs = l.graph.findEligibleParents()

	if len(tx.ParentIDs) == 0 {
		return tx, errors.New("no eligible parents available")
	}

	sort.Slice(tx.ParentIDs, func(i, j int) bool {
		return bytes.Compare(tx.ParentIDs[i][:], tx.ParentIDs[j][:]) < 0
	})

	for _, parentID := range tx.ParentIDs {
		parent, exists := l.graph.transactions[parentID]

		if !exists {
			return tx, errors.New("could not find transaction picked as an eligible parent")
		}

		if tx.Depth < parent.Depth {
			tx.Depth = parent.Depth
		}

		if tx.Confidence < parent.Confidence {
			tx.Confidence = parent.Confidence
		}
	}

	tx.Depth++
	tx.Confidence += uint64(len(tx.ParentIDs))

	tx.SenderSignature = edwards25519.Sign(l.keys.PrivateKey(), tx.Marshal())

	tx.rehash()

	return tx, nil
}

func (l *Ledger) addTransaction(tx Transaction) error {
	if err := l.graph.addTransaction(tx); err != nil {
		return err
	}

	ptr := l.graph.transactions[tx.ID]

	difficulty := l.rounds[l.round-1].Root.ExpectedDifficulty(byte(sys.MinDifficulty))

	if tx.IsCritical(difficulty) && l.snowball.Preferred() == nil {
		state, err := l.collapseTransactions(l.round, ptr, true)

		if err != nil {
			return errors.Wrap(err, "got an error collapsing down tx to get merkle root")
		}

		round := NewRound(l.round, state.Checksum(), tx)
		l.snowball.Prefer(&round)
	}

	return nil
}

func (l *Ledger) Run() {
	go l.recvLoop(l.ctx)
	go l.accounts.gcLoop(l.ctx)

	go l.gossipingLoop(l.ctx)
	go l.queryingLoop(l.ctx)

	go l.stateSyncingLoop(l.ctx)
}

func (l *Ledger) Stop() {
	l.cancel()
}

func (l *Ledger) Snapshot() *avl.Tree {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.accounts.snapshot()
}

func (l *Ledger) RoundID() uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.round
}

func (l *Ledger) LastRound() Round {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.rounds[l.round-1]
}

func (l *Ledger) Preferred() *Round {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.snowball.Preferred()
}

func (l *Ledger) NumTransactions() uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.getNumTransactions(l.round - 1)
}

func (l *Ledger) Height() uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.getHeight(l.round - 1)
}

func (l *Ledger) getNumTransactions(round uint64) uint64 {
	var n uint64

	height := l.graph.height

	if round+1 < l.round {
		height = l.rounds[round+1].Root.Depth + 1
	}

	for i := l.rounds[round].Root.Depth + 1; i < height; i++ {
		n += uint64(len(l.graph.depthIndex[i]))
	}

	return n
}

func (l *Ledger) getHeight(round uint64) uint64 {
	height := l.graph.height

	if round+1 < l.round {
		height = l.rounds[round+1].Root.Depth + 1
	}

	height -= l.rounds[round].Root.Depth

	if height > 0 {
		height--
	}

	return height
}

func (l *Ledger) FindTransaction(id common.TransactionID) (*Transaction, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	tx, exists := l.graph.transactions[id]
	return tx, exists
}

func (l *Ledger) ListTransactions(offset, limit uint64, sender, creator common.AccountID) (transactions []*Transaction) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, tx := range l.graph.transactions {
		if (sender == common.ZeroAccountID && creator == common.ZeroAccountID) || (sender != common.ZeroAccountID && tx.Sender == sender) || (creator != common.ZeroAccountID && tx.Creator == creator) {
			transactions = append(transactions, tx)
		}
	}

	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Depth < transactions[j].Depth
	})

	if offset != 0 || limit != 0 {
		if offset >= limit || offset >= uint64(len(transactions)) {
			return nil
		}

		if offset+limit > uint64(len(transactions)) {
			limit = uint64(len(transactions)) - offset
		}

		transactions = transactions[offset : offset+limit]
	}

	return
}

func (l *Ledger) recvLoop(ctx context.Context) {
	step := l.recv()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := step(ctx); err != nil {
			fmt.Println("recv error:", err)
		}
	}
}

func (l *Ledger) gossipingLoop(ctx context.Context) {
	step := l.gossip()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := step(ctx); err != nil {
			fmt.Println("gossip error:", err)
		}
	}
}

func (l *Ledger) queryingLoop(ctx context.Context) {
	step := l.query()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := step(ctx); err != nil {
			switch errors.Cause(err) {
			case ErrNonePreferred:
			default:
				fmt.Println("query error:", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

func (l *Ledger) stateSyncingLoop(ctx context.Context) {
	step := l.stateSync()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := step(ctx); err != nil {
			switch errors.Cause(err) {
			case ErrNonePreferred:
			default:
				fmt.Println("state sync error:", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
		}
	}
}

var ErrStopped = errors.New("worker stopped")

func (l *Ledger) recv() func(ctx context.Context) error {
	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return nil
		case evt := <-l.gossipIn:
			return func() error {
				l.mu.Lock()
				defer l.mu.Unlock()

				err := l.addTransaction(evt.TX)

				if err != nil {
					evt.Vote <- err
					return nil
				}

				evt.Vote <- nil

				return nil
			}()
		case evt := <-l.queryIn:
			return func() error {
				l.mu.Lock()
				defer l.mu.Unlock()

				r := evt.Round

				if r.Index < l.round { // Respond with the round we decided beforehand.
					round, available := l.rounds[r.Index]

					if !available {
						evt.Error <- errors.Errorf("got requested with round %d, but do not have it available", r.Index)
						return nil
					}

					evt.Response <- &round
					return nil
				}

				if err := l.addTransaction(r.Root); err != nil { // Add the root in the round to our graph.
					evt.Error <- err
					return nil
				}

				evt.Response <- l.snowball.Preferred() // Send back our preferred round info, if we have any.

				return nil
			}()
		case evt := <-l.outOfSyncIn:
			return func() error {
				l.mu.RLock()
				defer l.mu.RUnlock()

				if round, exists := l.rounds[l.round-1]; exists {
					evt.Response <- &round
				} else {
					evt.Response <- nil
				}

				return nil
			}()
		}
	}
}

func (l *Ledger) gossip() func(ctx context.Context) error {
	broadcastNops := false

	return func(ctx context.Context) error {
		snapshot := l.accounts.snapshot()

		var tx Transaction
		var err error

		var Result chan<- Transaction
		var Error chan<- error

		select {
		case <-ctx.Done():
			return nil
		case item := <-l.broadcastQueue:
			tx = Transaction{
				Tag:              item.Tag,
				Payload:          item.Payload,
				Creator:          item.Creator,
				CreatorSignature: item.Signature,
			}

			Result = item.Result
			Error = item.Error
		case <-time.After(1 * time.Millisecond):
			if !broadcastNops {
				select {
				case <-ctx.Done():
				case <-time.After(100 * time.Millisecond):
				}

				return nil
			}

			// Check if we have enough money available to create and broadcast a nop transaction.

			if balance, _ := ReadAccountBalance(snapshot, l.keys.PublicKey()); balance < sys.TransactionFeeAmount {
				select {
				case <-ctx.Done():
				case <-time.After(100 * time.Millisecond):
				}

				return nil
			}

			tx = NewTransaction(l.keys, sys.TagNop, nil)
		}

		tx, err = l.attachSenderToTransaction(tx)

		if err != nil {
			if Error != nil {
				Error <- errors.Wrap(err, "failed to sign off transaction")
			}
			return nil
		}

		evt := EventGossip{
			TX:     tx,
			Result: make(chan []VoteGossip, 1),
			Error:  make(chan error, 1),
		}

		select {
		case <-ctx.Done():
			if Error != nil {
				Error <- ErrStopped
			}

			return nil
		case <-time.After(1 * time.Second):
			if Error != nil {
				Error <- errors.New("gossip queue is full")
			}

			return nil
		case l.gossipOut <- evt:
		}

		var votes []VoteGossip

		select {
		case <-ctx.Done():
			if Error != nil {
				Error <- ErrStopped
			}

			return nil
		case err := <-evt.Error:
			if err != nil {
				if Error != nil {
					Error <- errors.Wrap(err, "got an error gossiping transaction out")
				}
				return nil
			}
		case <-time.After(1 * time.Second):
			if Error != nil {
				Error <- errors.New("did not get back a gossip response")
			}
		case votes = <-evt.Result:
		}

		if len(votes) == 0 {
			return nil
		}

		l.mu.Lock()
		defer l.mu.Unlock()

		accounts := make(map[common.AccountID]struct{})

		for _, vote := range votes {
			accounts[vote.Voter] = struct{}{}
		}

		weights := computeStakeDistribution(snapshot, accounts)

		positives := 0.0

		for _, vote := range votes {
			if vote.Ok {
				positives += weights[vote.Voter]
			}
		}

		if positives < sys.SnowballQueryAlpha {
			if Error != nil {
				Error <- errors.Errorf("only %.2f%% of queried peers find transaction %x valid", positives, evt.TX.ID)
			}

			return nil
		}

		// Double-check that after gossiping, we have not progressed a single view ID and
		// that the transaction is still valid for us to add to our view-graph.

		if err := l.addTransaction(tx); err != nil {
			if Error != nil {
				Error <- err
			}

			return nil
		}

		/** At this point, the transaction was successfully added to our view-graph. **/

		// If we have nothing else to broadcast and we are not broadcasting out
		// nop transactions, then start broadcasting out nop transactions.
		if len(l.broadcastQueue) == 0 && !broadcastNops && l.snowball.Preferred() == nil {
			broadcastNops = true
		}

		if l.snowball.Preferred() != nil {
			broadcastNops = false
		}

		if Result != nil {
			Result <- tx
		}

		return nil
	}
}

var ErrNonePreferred = errors.New("no critical transactions available in round yet")

// query atomically maintains the ledgers graph, and divides the graph from the bottom up into rounds.
//
// It maintains the ledgers Snowball instance while dividing up rounds, to see what the network believes
// is the preferred critical transaction selected to finalize the current ledgers round, and also be the
// root transaction for the next new round.
//
// It should be called repetitively as fast as possible in an infinite for loop, in a separate goroutine
// away from any other goroutines associated to the ledger.
func (l *Ledger) query() func(ctx context.Context) error {
	return func(ctx context.Context) error {
		oldRound := l.rounds[l.round-1]
		oldRoot := oldRound.Root

		if err := func() error {
			l.mu.Lock()
			defer l.mu.Unlock()

			if l.snowball.Preferred() == nil { // If we do not prefer any critical transaction yet, find a critical transaction to initially prefer first.
				difficulty := oldRoot.ExpectedDifficulty(byte(sys.MinDifficulty))

				var eligible []*Transaction // Find all critical transactions for the current round.

				for i := difficulty; i < math.MaxUint8; i++ {
					candidates, exists := l.graph.seedIndex[difficulty]

					if !exists {
						continue
					}

					for candidateID := range candidates {
						candidate := l.graph.transactions[candidateID]

						if candidate.Depth > oldRoot.Depth && candidate.IsCritical(difficulty) {
							eligible = append(eligible, candidate)
						}
					}
				}

				if len(eligible) == 0 { // If there are no critical transactions for the round yet, discontinue.
					return ErrNonePreferred
				}

				// Sort critical transactions by their depth, and pick the critical transaction
				// with the smallest depth as the nodes initial preferred transaction.
				//
				// The final selected critical transaction might change after a couple of
				// rounds with Snowball.

				sort.Slice(eligible, func(i, j int) bool {
					return eligible[i].Depth < eligible[j].Depth
				})

				proposed := eligible[0]

				state, err := l.collapseTransactions(l.round, proposed, true)

				if err != nil {
					return errors.Wrap(err, "got an error collapsing down tx to get merkle root")
				}

				initial := NewRound(l.round, state.Checksum(), *proposed)
				l.snowball.Prefer(&initial)
			}

			return nil
		}(); err != nil {
			return err
		}

		snapshot := l.accounts.snapshot()

		evt := EventQuery{
			Round:  l.snowball.Preferred(),
			Result: make(chan []VoteQuery, 1),
			Error:  make(chan error, 1),
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(1 * time.Second):
			return errors.New("query queue is full")
		case l.queryOut <- evt:
		}

		var votes []VoteQuery

		select {
		case <-ctx.Done():
			return nil
		case err := <-evt.Error:
			return errors.Wrap(err, "query got event error")
		case votes = <-evt.Result:
		}

		if len(votes) == 0 {
			return nil
		}

		l.mu.Lock()
		defer l.mu.Unlock()

		rounds := make(map[common.RoundID]Round)
		accounts := make(map[common.AccountID]struct{}, len(votes))

		for _, vote := range votes {
			if vote.Preferred.Index == l.round && vote.Preferred.ID != common.ZeroRoundID && vote.Preferred.Root.ID != common.ZeroTransactionID {
				rounds[vote.Preferred.ID] = vote.Preferred
			}

			accounts[vote.Voter] = struct{}{}
		}

		var elected *Round
		counts := make(map[common.RoundID]float64)

		weights := computeStakeDistribution(snapshot, accounts)

		for _, vote := range votes {
			if vote.Preferred.Index == l.round && vote.Preferred.Root.ID != common.ZeroTransactionID {
				counts[vote.Preferred.ID] += weights[vote.Voter]

				if counts[vote.Preferred.ID] >= sys.SnowballQueryAlpha {
					elected = &vote.Preferred
					break
				}
			}
		}

		l.snowball.Tick(elected)

		if l.snowball.Decided() {
			newRound := l.snowball.Preferred()
			newRoot := l.graph.transactions[newRound.Root.ID]

			state, err := l.collapseTransactions(newRound.Index, newRoot, true)

			if err != nil {
				return errors.Wrap(err, "got an error finalizing a round")
			}

			if state.Checksum() != newRound.Merkle {
				return errors.Errorf("expected finalized rounds merkle root to be %x, but got %x", newRound.Merkle, state.Checksum())
			}

			if err = l.accounts.commit(state); err != nil {
				return errors.Wrap(err, "failed to commit collapsed state to our database")
			}

			l.snowball.Reset()
			l.graph.Reset(newRound)

			l.prune(newRound)

			logger := log.Consensus("round_end")
			logger.Info().
				Uint64("num_tx", l.getNumTransactions(l.round-1)).
				Uint64("old_round", oldRound.Index).
				Uint64("new_round", newRound.Index).
				Uint8("old_difficulty", oldRound.Root.ExpectedDifficulty(byte(sys.MinDifficulty))).
				Uint8("new_difficulty", newRound.Root.ExpectedDifficulty(byte(sys.MinDifficulty))).
				Hex("new_root", newRoot.ID[:]).
				Hex("old_root", oldRoot.ID[:]).
				Hex("new_merkle_root", newRound.Merkle[:]).
				Hex("old_merkle_root", oldRound.Merkle[:]).
				Msg("Finalized consensus round, and initialized a new round.")

			l.rounds[l.round] = *newRound
			l.round++
		}

		return nil
	}
}

func (l *Ledger) stateSync() func(ctx context.Context) error {
	return func(ctx context.Context) error {
		snapshot := l.accounts.snapshot()

		evt := EventOutOfSyncCheck{
			Result: make(chan []VoteOutOfSync, 1),
			Error:  make(chan error, 1),
		}

		select {
		case <-ctx.Done():
			return nil
		case l.outOfSyncOut <- evt:
		}

		var votes []VoteOutOfSync

		select {
		case <-ctx.Done():
			return nil
		case err := <-evt.Error:
			return errors.Wrap(ErrNonePreferred, err.Error())
		case votes = <-evt.Result:
		}

		if len(votes) == 0 {
			return nil
		}

		rounds := make(map[common.RoundID]Round)
		accounts := make(map[common.AccountID]struct{}, len(votes))

		for _, vote := range votes {
			if vote.Round.ID != common.ZeroRoundID && vote.Round.Root.ID != common.ZeroTransactionID {
				rounds[vote.Round.ID] = vote.Round
			}

			accounts[vote.Voter] = struct{}{}
		}

		var elected *Round
		counts := make(map[common.RoundID]float64)

		l.mu.Lock()
		defer l.mu.Unlock()

		weights := computeStakeDistribution(snapshot, accounts)

		for _, vote := range votes {
			if vote.Round.Root.ID != common.ZeroTransactionID {
				counts[vote.Round.ID] += weights[vote.Voter]

				if counts[vote.Round.ID] >= sys.SnowballSyncAlpha {
					elected = &vote.Round
					break
				}
			}
		}

		l.resolver.Tick(elected)

		if l.resolver.Decided() {
			proposedRound := l.resolver.Preferred()

			// The round number we came to consensus to being the latest within the network
			// is less than or equal to ours. Go back to square one.

			if l.round-1 >= proposedRound.Index {
				l.resolver.Reset()
				return ErrNonePreferred
			}

			fmt.Printf("We are out of sync; our current round is %d, and our peers proposed round is %d.\n", l.round-1, proposedRound.Index)

			return ErrNonePreferred
		}

		return nil
	}
}

// prune prunes away all transactions and indices with a view ID < (current view ID - pruningDepth).
func (l *Ledger) prune(round *Round) {
	for roundID, transactions := range l.graph.roundIndex {
		if roundID+pruningDepth <= round.Index {
			for id := range transactions {
				l.graph.deleteTransaction(id)
			}

			delete(l.graph.roundIndex, roundID)

			logger := log.Consensus("prune")
			logger.Debug().
				Int("num_tx", len(l.graph.roundIndex[round.Index])).
				Uint64("current_round_id", round.Index).
				Uint64("pruned_round_id", roundID).
				Msg("Pruned away round and its corresponding transactions.")
		}
	}

	for roundID := range l.rounds {
		if roundID+pruningDepth <= round.Index {
			delete(l.rounds, roundID)
		}
	}
}

// collapseTransactions takes all transactions recorded in a single round so far, and applies
// all valid and available ones to a snapshot of all accounts stored in the ledger.
//
// It returns an updated accounts snapshot after applying all finalized transactions.
func (l *Ledger) collapseTransactions(round uint64, tx *Transaction, logging bool) (*avl.Tree, error) {
	snapshot := l.accounts.snapshot()
	snapshot.SetViewID(round + 1)

	root := l.rounds[l.round-1].Root

	visited := map[common.TransactionID]struct{}{
		root.ID: {},
	}

	aq := AcquireQueue()
	defer ReleaseQueue(aq)

	for _, parentID := range tx.ParentIDs {
		if parentID == root.ID {
			continue
		}

		visited[parentID] = struct{}{}

		if parent, exists := l.graph.transactions[parentID]; exists {
			aq.PushBack(parent)
		} else {
			return snapshot, errors.Errorf("missing parent to correctly collapse down ledger state from critical transaction %x", tx.ID)
		}
	}

	bq := AcquireQueue()
	defer ReleaseQueue(bq)

	for aq.Len() > 0 {
		popped := aq.PopFront().(*Transaction)

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				visited[parentID] = struct{}{}

				if parent, exists := l.graph.transactions[parentID]; exists {
					aq.PushBack(parent)
				} else {
					return snapshot, errors.Errorf("missing ancestor to correctly collapse down ledger state from critical transaction %x", tx.ID)
				}
			}
		}

		bq.PushBack(popped)
	}

	// Apply transactions in reverse order from the root of the view-graph all the way up to the newly
	// created critical transaction.
	for bq.Len() > 0 {
		popped := bq.PopBack().(*Transaction)

		// If any errors occur while applying our transaction to our accounts
		// snapshot, silently log it and continue applying other transactions.
		if err := l.rewardValidators(snapshot, popped, logging); err != nil {
			if logging {
				logger := log.Node()
				logger.Warn().Err(err).Msg("Failed to reward a validator while collapsing down transactions.")
			}
		}

		if err := l.applyTransactionToSnapshot(snapshot, popped); err != nil {
			if logging {
				logger := log.TX(popped.ID, popped.Sender, popped.Creator, popped.Nonce, popped.ParentIDs, popped.Tag, popped.Payload, "failed")
				logger.Log().Err(err).Msg("Failed to apply transaction to the ledger.")
			}
		} else {
			if logging {
				logger := log.TX(popped.ID, popped.Sender, popped.Creator, popped.Nonce, popped.ParentIDs, popped.Tag, popped.Payload, "applied")
				logger.Log().Msg("Successfully applied transaction to the ledger.")
			}
		}

		// Update nonce.
		nonce, _ := ReadAccountNonce(snapshot, popped.Creator)
		WriteAccountNonce(snapshot, popped.Creator, nonce+1)
	}

	//l.cacheAccounts.put(tx.getCriticalSeed(), snapshot)
	return snapshot, nil
}

func (l *Ledger) applyTransactionToSnapshot(ss *avl.Tree, tx *Transaction) error {
	ctx := newTransactionContext(ss, tx)

	if err := ctx.apply(l.processors); err != nil {
		return errors.Wrap(err, "could not apply transaction to snapshot")
	}

	return nil
}

func (l *Ledger) rewardValidators(ss *avl.Tree, tx *Transaction, logging bool) error {
	var candidates []*Transaction
	var stakes []uint64
	var totalStake uint64

	visited := make(map[common.AccountID]struct{})

	q := AcquireQueue()
	defer ReleaseQueue(q)

	for _, parentID := range tx.ParentIDs {
		if parent, exists := l.graph.transactions[parentID]; exists {
			q.PushBack(parent)
		}

		visited[parentID] = struct{}{}
	}

	// Ignore error; should be impossible as not using HMAC mode.
	hasher, _ := blake2b.New256(nil)

	var depthCounter uint64
	var lastDepth = tx.Depth

	for q.Len() > 0 {
		popped := q.PopFront().(*Transaction)

		if popped.Depth != lastDepth {
			lastDepth = popped.Depth
			depthCounter++
		}

		// If we exceed the max eligible depth we search for candidate
		// validators to reward from, stop traversing.
		if depthCounter >= sys.MaxEligibleParentsDepthDiff {
			break
		}

		// Filter for all ancestral transactions not from the same sender,
		// and within the desired graph depth.
		if popped.Sender != tx.Sender {
			stake, _ := ReadAccountStake(ss, popped.Sender)

			if stake > sys.MinimumStake {
				candidates = append(candidates, popped)
				stakes = append(stakes, stake)

				totalStake += stake

				// Record entropy source.
				if _, err := hasher.Write(popped.ID[:]); err != nil {
					return errors.Wrap(err, "stake: failed to hash transaction id for entropy source")
				}
			}
		}

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				if parent, exists := l.graph.transactions[parentID]; exists {
					q.PushBack(parent)
				}

				visited[parentID] = struct{}{}
			}
		}
	}

	// If there are no eligible rewardee candidates, do not reward anyone.
	if len(candidates) == 0 || len(stakes) == 0 || totalStake == 0 {
		return nil
	}

	entropy := hasher.Sum(nil)
	acc, threshold := float64(0), float64(binary.LittleEndian.Uint64(entropy)%uint64(0xffff))/float64(0xffff)

	var rewardee *Transaction

	// Model a weighted uniform distribution by a random variable X, and select
	// whichever validator has a weight X ≥ X' as a reward recipient.
	for i, tx := range candidates {
		acc += float64(stakes[i]) / float64(totalStake)

		if acc >= threshold {
			rewardee = tx
			break
		}
	}

	// If there is no selected transaction that deserves a reward, give the
	// reward to the last reward candidate.
	if rewardee == nil {
		rewardee = candidates[len(candidates)-1]
	}

	senderBalance, _ := ReadAccountBalance(ss, tx.Sender)
	recipientBalance, _ := ReadAccountBalance(ss, rewardee.Sender)

	fee := sys.TransactionFeeAmount

	if senderBalance < fee {
		return errors.Errorf("stake: sender %x does not have enough PERLs to pay transaction fees (requested %d PERLs) to %x", tx.Sender, fee, rewardee.Sender)
	}

	WriteAccountBalance(ss, tx.Sender, senderBalance-fee)
	WriteAccountBalance(ss, rewardee.Sender, recipientBalance+fee)

	if logging {
		logger := log.Stake("reward_validator")
		logger.Info().
			Hex("sender", tx.Sender[:]).
			Hex("recipient", rewardee.Sender[:]).
			Hex("sender_tx_id", tx.ID[:]).
			Hex("rewardee_tx_id", rewardee.ID[:]).
			Hex("entropy", entropy).
			Float64("acc", acc).
			Float64("threshold", threshold).Msg("Rewarded validator.")
	}

	return nil
}
