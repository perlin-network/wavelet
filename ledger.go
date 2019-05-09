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
	"runtime"
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
	wg     sync.WaitGroup

	keys *skademlia.Keypair

	metrics  *Metrics
	accounts *Accounts
	graph    *Graph

	snowball *Snowball
	resolver *Snowball

	verifier *verifier

	processors map[byte]TransactionProcessor

	rounds map[uint64]Round
	round  uint64

	kv store.KV

	mu sync.RWMutex

	syncing     bool
	syncingCond sync.Cond

	BroadcastQueue chan<- EventBroadcast
	broadcastQueue <-chan EventBroadcast

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

	DownloadTxIn chan<- EventIncomingDownloadTX
	downloadTxIn <-chan EventIncomingDownloadTX

	DownloadTxOut <-chan EventDownloadTX
	downloadTxOut chan<- EventDownloadTX

	LatestViewIn chan<- EventIncomingLatestView
	latestViewIn <-chan EventIncomingLatestView

	LatestViewOut <-chan EventLatestView
	latestViewOut chan<- EventLatestView

	GossipTxIn chan<- EventGossip
	gossipTxIn <-chan EventGossip

	GossipTxOut <-chan EventGossip
	gossipTxOut chan<- EventGossip
}

func NewLedger(keys *skademlia.Keypair, kv store.KV, genesis *string) *Ledger {
	ctx, cancel := context.WithCancel(context.Background())

	broadcastQueue := make(chan EventBroadcast, 1024)

	queryIn := make(chan EventIncomingQuery, 128)
	queryOut := make(chan EventQuery, 128)

	outOfSyncIn := make(chan EventIncomingOutOfSyncCheck, 16)
	outOfSyncOut := make(chan EventOutOfSyncCheck, 16)

	syncInitIn := make(chan EventIncomingSyncInit, 16)
	syncInitOut := make(chan EventSyncInit, 16)

	syncDiffIn := make(chan EventIncomingSyncDiff, 128)
	syncDiffOut := make(chan EventSyncDiff, 128)

	downloadTxIn := make(chan EventIncomingDownloadTX, 16)
	downloadTxOut := make(chan EventDownloadTX, 16)

	latestViewIn := make(chan EventIncomingLatestView, 16)
	latestViewOut := make(chan EventLatestView, 16)

	gossipTxIn := make(chan EventGossip, 1024)
	gossipTxOut := make(chan EventGossip, 1024)

	accounts := newAccounts(kv)

	var round Round
	var roundCount uint64

	savedRound, savedCount, err := loadRound(kv)
	if err != nil {
		round = performInception(accounts.tree, genesis)
		if err := accounts.commit(nil); err != nil {
			panic(err)
		}
		roundCount = 1
	} else {
		round = *savedRound
		roundCount = savedCount
	}

	graph := NewGraph(&round)

	metrics := NewMetrics()

	return &Ledger{
		ctx:    ctx,
		cancel: cancel,

		keys: keys,

		metrics:  metrics,
		accounts: accounts,
		graph:    graph,

		snowball: NewSnowball().WithK(sys.SnowballK).WithAlpha(sys.SnowballAlpha).WithBeta(sys.SnowballBeta),
		resolver: NewSnowball().WithK(sys.SnowballK).WithAlpha(sys.SnowballAlpha).WithBeta(sys.SnowballBeta),

		verifier: NewVerifier(runtime.NumCPU(), 1024),

		processors: map[byte]TransactionProcessor{
			sys.TagNop:      ProcessNopTransaction,
			sys.TagTransfer: ProcessTransferTransaction,
			sys.TagContract: ProcessContractTransaction,
			sys.TagStake:    ProcessStakeTransaction,
		},

		rounds: map[uint64]Round{round.Index: round},
		round:  roundCount,

		kv: kv,

		syncingCond: sync.Cond{L: new(sync.Mutex)},

		BroadcastQueue: broadcastQueue,
		broadcastQueue: broadcastQueue,

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

		DownloadTxIn: downloadTxIn,
		downloadTxIn: downloadTxIn,

		DownloadTxOut: downloadTxOut,
		downloadTxOut: downloadTxOut,

		LatestViewIn: latestViewIn,
		latestViewIn: latestViewIn,

		LatestViewOut: latestViewOut,
		latestViewOut: latestViewOut,

		GossipTxIn: gossipTxIn,
		gossipTxIn: gossipTxIn,

		GossipTxOut: gossipTxOut,
		gossipTxOut: gossipTxOut,
	}
}

func NewTransaction(creator *skademlia.Keypair, tag byte, payload []byte) Transaction {
	tx := Transaction{Tag: tag, Payload: payload}

	var nonce [8]byte // TODO(kenta): nonce

	tx.Creator = creator.PublicKey()
	tx.CreatorSignature = edwards25519.Sign(creator.PrivateKey(), append(nonce[:], append([]byte{tx.Tag}, tx.Payload...)...))

	return tx
}

func (l *Ledger) attachSenderToTransaction(tx Transaction) (Transaction, error) {
	tx.Sender = l.keys.PublicKey()
	tx.ParentIDs = l.graph.FindEligibleParents()

	if len(tx.ParentIDs) == 0 {
		return tx, errors.New("no eligible parents available")
	}

	sort.Slice(tx.ParentIDs, func(i, j int) bool {
		return bytes.Compare(tx.ParentIDs[i][:], tx.ParentIDs[j][:]) < 0
	})

	for _, parentID := range tx.ParentIDs {
		parent, exists := l.graph.LookupTransactionByID(parentID)

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
	if err := l.verifier.verify(&tx); err != nil {
		return err
	}

	if err := l.graph.AddTransaction(tx); err == nil {
		if tx.Sender != l.keys.PublicKey() {
			select {
			case l.gossipTxOut <- EventGossip{TX: tx}:
			default:
			}
		}

		l.metrics.receivedTX.Mark(1)
	} else if err != ErrAlreadyExists {
		return err
	}

	return nil
}

func (l *Ledger) Run() {
	for i := 0; i < 4; i++ {
		go l.recvLoop(l.ctx)
	}

	go l.accounts.gcLoop(l.ctx)

	go l.gossipingLoop(l.ctx)
	go l.queryingLoop(l.ctx)

	go l.stateSyncingLoop(l.ctx)
	go l.txSyncingLoop(l.ctx)

	go l.metrics.runLogger(l.ctx)
}

func (l *Ledger) Stop() {
	l.cancel()
	l.wg.Wait()

	l.metrics.Stop()

	l.verifier.Stop()
}

func (l *Ledger) Snapshot() *avl.Tree {
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
	return l.snowball.Preferred()
}

func (l *Ledger) NumTransactions() uint64 {
	return l.getNumTransactions(l.round - 1)
}

func (l *Ledger) NumTransactionInStore() uint64 {
	return l.graph.NumTransactionsInStore()
}

func (l *Ledger) NumMissingTransactions() uint64 {
	return l.graph.NumMissingTransactions()
}

func (l *Ledger) Height() uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.getHeight(l.round - 1)
}

func (l *Ledger) getNumTransactions(round uint64) uint64 {
	var n uint64

	height := l.graph.Height()

	if round+1 < l.round {
		height = l.rounds[round+1].Root.Depth + 1
	}

	for depth := l.rounds[round].Root.Depth + 1; depth < height; depth++ {
		n += l.graph.NumTransactionsInDepth(depth)
	}

	return n
}

func (l *Ledger) getHeight(round uint64) uint64 {
	height := l.graph.Height()

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
	tx := l.graph.GetTransaction(id)

	if tx == nil {
		return nil, false
	}

	return tx, true
}

func (l *Ledger) TransactionApplied(id common.TransactionID) bool {
	return l.graph.TransactionApplied(id)
}

func (l *Ledger) ListTransactions(offset, limit uint64, sender, creator common.AccountID) (transactions []*Transaction) {
	return l.graph.ListTransactions(offset, limit, sender, creator)
}

func (l *Ledger) recvLoop(ctx context.Context) {
	l.wg.Add(1)
	defer l.wg.Done()

	step := recv(l)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := step(ctx); err != nil && errors.Cause(err) != ErrMissingParents {
			fmt.Println("recv error:", err)
		}
	}
}

func (l *Ledger) gossipingLoop(ctx context.Context) {
	l.wg.Add(1)
	defer l.wg.Done()

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
	l.wg.Add(1)
	defer l.wg.Done()

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
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

func (l *Ledger) stateSyncingLoop(ctx context.Context) {
	l.wg.Add(1)
	defer l.wg.Done()

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
	l.wg.Add(1)
	defer l.wg.Done()

	step := txSync(l)

L:
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := step(ctx); err != nil {
			switch errors.Cause(err) {
			case ErrNonePreferred:
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Millisecond):
				}

				continue L
			default:
			}

			fmt.Println("tx sync error:", err)
		}
	}
}

// prune prunes away all transactions and indices with a view ID < (current view ID - PruningDepth).
func (l *Ledger) prune(round *Round) {
	l.graph.Prune(round)

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

	root := l.LastRound().Root

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

		if parent, exists := l.graph.LookupTransactionByID(parentID); exists {
			if parent.Depth > root.Depth {
				aq.PushBack(parent)
			}
		} else {
			return snapshot, errors.Errorf("missing parent %x to correctly collapse down ledger state from critical transaction %x", parentID, tx.ID)
		}
	}

	bq := AcquireQueue()
	defer ReleaseQueue(bq)

	for aq.Len() > 0 {
		popped := aq.PopFront().(*Transaction)

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				visited[parentID] = struct{}{}

				if parent, exists := l.graph.LookupTransactionByID(parentID); exists {
					if parent.Depth > root.Depth {
						aq.PushBack(parent)
					}
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
		if err := l.rewardValidators(snapshot, root, popped, logging); err != nil {
			if logging {
				logger := log.TX(popped.ID, popped.Sender, popped.Creator, popped.Nonce, popped.Depth, popped.Confidence, popped.ParentIDs, popped.Tag, popped.Payload, "failed")
				logger.Log().Err(err).Msg("Failed to deduct transaction fees and reward validators before applying the transaction to the ledger.")
			}

			continue
		}

		if err := l.applyTransactionToSnapshot(snapshot, popped); err != nil {
			if logging {
				logger := log.TX(popped.ID, popped.Sender, popped.Creator, popped.Nonce, popped.Depth, popped.Confidence, popped.ParentIDs, popped.Tag, popped.Payload, "failed")
				logger.Log().Err(err).Msg("Failed to apply transaction to the ledger.")
			}

			continue
		}

		if logging {
			logger := log.TX(popped.ID, popped.Sender, popped.Creator, popped.Nonce, popped.Depth, popped.Confidence, popped.ParentIDs, popped.Tag, popped.Payload, "applied")
			logger.Log().Msg("Successfully applied transaction to the ledger.")
		}

		// Update nonce.
		nonce, _ := ReadAccountNonce(snapshot, popped.Creator)
		WriteAccountNonce(snapshot, popped.Creator, nonce+1)

		if logging {
			l.metrics.acceptedTX.Mark(1)
		}
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

func (l *Ledger) rewardValidators(ss *avl.Tree, root Transaction, tx *Transaction, logging bool) error {
	var candidates []*Transaction
	var stakes []uint64
	var totalStake uint64

	visited := make(map[common.TransactionID]struct{})

	q := AcquireQueue()
	defer ReleaseQueue(q)

	for _, parentID := range tx.ParentIDs {
		if parent, exists := l.graph.LookupTransactionByID(parentID); exists {
			if parent.Depth > root.Depth {
				q.PushBack(parent)
			}
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
		if depthCounter >= sys.MaxDepthDiff {
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
				if parent, exists := l.graph.LookupTransactionByID(parentID); exists {
					if parent.Depth > root.Depth {
						q.PushBack(parent)
					}
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

	creatorBalance, _ := ReadAccountBalance(ss, tx.Creator)
	recipientBalance, _ := ReadAccountBalance(ss, rewardee.Sender)

	fee := sys.TransactionFeeAmount

	if creatorBalance < fee {
		return errors.Errorf("stake: creator %x does not have enough PERLs to pay transaction fees (requested %d PERLs) to %x", tx.Creator, fee, rewardee.Sender)
	}

	WriteAccountBalance(ss, tx.Creator, creatorBalance-fee)
	WriteAccountBalance(ss, rewardee.Sender, recipientBalance+fee)

	if logging {
		logger := log.Stake("reward_validator")
		logger.Info().
			Hex("creator", tx.Creator[:]).
			Hex("recipient", rewardee.Sender[:]).
			Hex("creator_tx_id", tx.ID[:]).
			Hex("rewardee_tx_id", rewardee.ID[:]).
			Hex("entropy", entropy).
			Float64("acc", acc).
			Float64("threshold", threshold).Msg("Rewarded validator.")
	}

	return nil
}

func storeRound(kv store.KV, count uint64, round Round) error {
	// TODO(kenta): old rounds need to be pruned from the store as well

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], count)

	var err error
	if err = kv.Put(keyRoundCount[:], buf[:]); err != nil {
		return err
	}

	return kv.Put(keyRoundLatest[:], round.Marshal())
}

func loadRound(kv store.KV) (*Round, uint64, error) {
	var b []byte
	var err error

	var count uint64
	b, err = kv.Get(keyRoundCount[:])
	if err != nil {
		return nil, 0, err
	}
	count = binary.BigEndian.Uint64(b[:8])

	var round Round
	b, err = kv.Get(keyRoundLatest[:])
	if err != nil {
		return nil, 0, err
	}
	round, err = UnmarshalRound(bytes.NewReader(b))
	if err != nil {
		return nil, 0, err
	}

	return &round, count, nil
}
