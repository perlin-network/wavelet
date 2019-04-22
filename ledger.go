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
	"sort"
	"sync"
	"time"
)

var (
	ErrStopped       = errors.New("worker stopped")
	ErrNonePreferred = errors.New("no critical transactions available in round yet")
)

const PruningDepth = 30

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

	syncing     bool
	syncingCond sync.Cond

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

	SyncTxIn chan<- EventIncomingDownloadTX
	syncTxIn <-chan EventIncomingDownloadTX

	SyncTxOut     <-chan EventDownloadTX
	downloadTxOut chan<- EventDownloadTX

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

	syncTxIn := make(chan EventIncomingDownloadTX, 16)
	syncTxOut := make(chan EventDownloadTX, 16)

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

		syncingCond: sync.Cond{L: new(sync.Mutex)},

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

		SyncTxOut:     syncTxOut,
		downloadTxOut: syncTxOut,

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
		parent, exists := l.graph.lookupTransactionByID(parentID)

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
			return errors.Wrap(err, "failed to collapse down critical transaction which we have received")
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
	go l.txSyncingLoop(l.ctx)
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
	step := recv(l)

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
	step := gossip(l)

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
	step := query(l)

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
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}

func (l *Ledger) stateSyncingLoop(ctx context.Context) {
	step := stateSync(l)

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
		} else {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}

func (l *Ledger) txSyncingLoop(ctx context.Context) {
	step := txSync(l)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := step(ctx); err != nil {
			fmt.Println("tx sync error:", err)
		}
	}
}

// prune prunes away all transactions and indices with a view ID < (current view ID - PruningDepth).
func (l *Ledger) prune(round *Round) {
	for roundID, transactions := range l.graph.roundIndex {
		if roundID+PruningDepth <= round.Index {
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
		if roundID+PruningDepth <= round.Index {
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

		if parent, exists := l.graph.lookupTransactionByID(parentID); exists {
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

				if parent, exists := l.graph.lookupTransactionByID(parentID); exists {
					aq.PushBack(parent)
				} else {
					return snapshot, errors.Errorf("missing ancestor %x to correctly collapse down ledger state from critical transaction %x", parentID, tx.ID)
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
		if parent, exists := l.graph.lookupTransactionByID(parentID); exists {
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
				if parent, exists := l.graph.lookupTransactionByID(parentID); exists {
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
