package wavelet

import (
	"github.com/perlin-network/noise/identity"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet/conflict"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"golang.org/x/crypto/blake2b"
	"time"
)

var (
	ErrTxNotCritical = errors.New("wavelet: tx is not critical")
)

type Ledger struct {
	accounts

	kv store.KV

	resolver conflict.Resolver
	view     graph

	processors map[byte]TransactionProcessor

	viewID     atomic.Uint64
	difficulty atomic.Uint64
}

func NewLedger(kv store.KV, genesisPath string) *Ledger {
	ledger := &Ledger{
		accounts: newAccounts(kv),

		kv: kv,

		resolver: conflict.NewSnowball().
			WithK(sys.SnowballK).
			WithAlpha(sys.SnowballAlpha).
			WithBeta(sys.SnowballBeta),

		processors: make(map[byte]TransactionProcessor),
	}

	ledger.difficulty.Store(sys.MinimumDifficulty)

	genesis, err := performInception(ledger.accounts, genesisPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to perform inception with genesis data from %q.", genesisPath)
	}

	// Instantiate the view-graph for our ledger.
	ledger.view = newGraph(genesis)

	return ledger
}

// NewTransaction uses our ledger to create a transaction with a specified
// tag and payload, and uses a key pair to sign the transaction attach the
// signature to the transaction.
//
// Afterwards, it attaches a sender (see *Ledger.AttachSenderToTransaction(...))
// such that the transaction is ready to be broadcasted out to the network.
func (l *Ledger) NewTransaction(keys identity.Keypair, tag byte, payload []byte) (*Transaction, error) {
	// Perform 'creator' portion of a transaction.
	tx := &Transaction{
		Tag:     tag,
		Payload: payload,
	}

	copy(tx.Creator[:], keys.PublicKey())

	creatorSignature, err := eddsa.Sign(keys.PrivateKey(), append([]byte{tag}, payload...))
	if err != nil {
		return nil, errors.Wrap(err, "failed to make creator signature")
	}

	copy(tx.CreatorSignature[:], creatorSignature)

	// Perform 'sender' portion of a transaction.
	err = l.AttachSenderToTransaction(keys, tx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to attach sender to transaction")
	}

	return tx, nil
}

// AttachSenderToTransaction uses our ledger to derive for a transaction its
// parents, timestamp, accounts merkle root should the transaction be a critical
// transaction.
//
// Afterwards, it uses a key pair under our ledger to sign the transaction, attach
// the signature to the transaction, and re-hashes the ID of the transaction.
//
// It returns an error if the key pair provided is ineligible to be used for signing
// transactions under the EdDSA signature scheme.
func (l *Ledger) AttachSenderToTransaction(keys identity.Keypair, tx *Transaction) error {
	copy(tx.Sender[:], keys.PublicKey())

	tx.ParentIDs = l.view.findEligibleParents()
	tx.Timestamp = uint64(time.Duration(time.Now().UnixNano()) / time.Millisecond)

	if tx.IsCritical(l.Difficulty()) {
		snapshot := l.collapseTransactions()
		tx.AccountsMerkleRoot = snapshot.tree.Checksum()
	}

	senderSignature, err := eddsa.Sign(keys.PrivateKey(), tx.Write())
	if err != nil {
		return errors.Wrap(err, "failed to make sender signature")
	}

	copy(tx.SenderSignature[:], senderSignature)

	tx.rehash()

	return nil
}

func (l *Ledger) ReceiveTransaction(tx *Transaction) error {
	if !l.assertValidTimestamp(tx) {
		return errors.Wrap(VoteRejected, "wavelet: either tx timestamp is out of bounds, or parents not available")
	}

	err := l.view.addTransaction(tx)

	// Reject transaction if the parents are not available.
	switch errors.Cause(err) {
	case ErrParentsNotAvailable:
		return errors.Wrap(VoteRejected, "wavelet: parents are not available")
	case ErrTxAlreadyExists:
	}

	if !l.assertValidParentDepths(tx) {
		return errors.Wrap(VoteRejected, "wavelet: parent depths are out of bounds")
	}

	return VoteAccepted
}

func (l *Ledger) assertValidParentDepths(tx *Transaction) bool {
	for _, parentID := range tx.ParentIDs {
		parent, stored := l.view.transactions[parentID]

		if !stored {
			return false
		}

		if parent.depth+sys.MaxEligibleParentsDepthDiff < tx.depth {
			return false
		}
	}

	return true
}

func (l *Ledger) assertValidTimestamp(tx *Transaction) bool {
	visited := make(map[[blake2b.Size256]byte]struct{})
	queue := queue.New()

	for _, parentID := range tx.ParentIDs {
		parent, stored := l.view.transactions[parentID]

		if !stored {
			return false
		}

		queue.PushBack(parent)
	}

	var timestamps []uint64

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		timestamps = append(timestamps, popped.Timestamp)

		if popped == l.view.root || len(timestamps) == sys.MedianTimestampNumAncestors {
			break
		}

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				parent, stored := l.view.transactions[parentID]

				if !stored {
					return false
				}

				queue.PushBack(parent)
			}
		}

		visited[popped.ID] = struct{}{}
	}

	median := computeMedianTimestamp(timestamps)

	// Check that the transactions timestamp is within the range:
	//
	// TIMESTAMP ∈ (median(last 10 BFS-ordered transactions in terms of history), nodes current time + 2 hours]
	if tx.Timestamp <= median {
		return false
	}

	if tx.Timestamp > uint64(time.Duration(time.Now().Add(2*time.Hour).UnixNano())/time.Millisecond) {
		return false
	}

	return true
}

func (l *Ledger) ReceiveQuery(tx *Transaction, responses map[[blake2b.Size256]byte]bool) error {
	if !tx.IsCritical(l.Difficulty()) {
		return ErrTxNotCritical
	}

	// Weigh votes based on each voters stake.
	var stakes []uint64
	var maxStake uint64

	for accountID, response := range responses {
		stake, _ := l.ReadAccountStake(accountID)

		if !response {
			stake = 0
		} else if stake < sys.MinimumStake {
			stake = sys.MinimumStake
		}

		if maxStake < stake {
			maxStake = stake
		}

		stakes = append(stakes, stake)
	}

	var votes []float64

	for _, stake := range stakes {
		votes = append(votes, float64(stake)/float64(maxStake))
	}

	log.Debug().Floats64("weighed_votes", votes).Msg("Weighed votes with stakes, and queued critical transaction for consensus.")

	// Update conflict resolver.
	l.resolver.Tick(tx.ID, votes)

	// If a consensus has been decided on the next critical transaction, then reset
	// the view-graph, increment the current view ID, and update the current ledgers
	// difficulty.
	if l.resolver.Decided() {
		old := l.view.root

		rootID := l.resolver.Result()
		root, recorded := l.view.transactions[rootID]

		if !recorded {
			return errors.New("wavelet: could not find newly critical tx in view graph")
		}

		ss := l.collapseTransactions()
		ss.snapshot = false

		l.accounts = ss

		err := l.CommitAccounts()
		if err != nil {
			return errors.Wrap(err, "wavelet: failed to collapse and commit new ledger state to db")
		}

		l.view.reset(root)
		viewID := l.viewID.Add(1)

		err = l.adjustDifficulty(root)
		if err != nil {
			return errors.Wrap(err, "wavelet: failed to adjust difficulty")
		}

		l.resolver.Reset()

		log.Info().
			Uint64("old_view_id", viewID-1).
			Uint64("new_view_id", viewID).
			Hex("new_root", rootID[:]).
			Hex("old_root", old.ID[:]).
			Msg("Finalized consensus round, and incremented view ID.")
	}

	return nil
}

func (l *Ledger) adjustDifficulty(critical *Transaction) error {
	var timestamps []uint64

	// Load critical timestamp history if it exists.
	buf, err := l.kv.Get(keyCriticalTimestampHistory[:])
	if err == nil {
		reader := payload.NewReader(buf)

		size, err := reader.ReadByte()
		if err != nil {
			panic(errors.Wrap(err, "wavelet: failed to read length of critical timestamp history"))
		}

		for i := 0; i < int(size); i++ {
			timestamp, err := reader.ReadUint64()
			if err != nil {
				panic(errors.Wrap(err, "wavelet: failed to read a single historical critical timestamp"))
			}

			timestamps = append(timestamps, timestamp)
		}
	}

	timestamps = append(timestamps, critical.Timestamp)

	// Prune away critical timestamp history if needed.
	if len(timestamps) > sys.MaxCriticalTimestampHistoryKept {
		timestamps = timestamps[len(timestamps)-sys.MaxCriticalTimestampHistoryKept:]
	}

	mean := computeMeanTimestamp(timestamps)

	// Adjust the current ledgers difficulty to:
	//
	// DIFFICULTY = A_0 + (LAST_DIFFICULTY - A_0) * (10 seconds / average(time it took
	// to create critical transaction for the last 10 critical transactions))
	l.difficulty.Store(sys.MinimumDifficulty + (l.difficulty.Load()-sys.MinimumDifficulty)*
		(sys.ExpectedConsensusTimeMilliseconds/mean))

	// Save critical timestamp history to disk.
	writer := payload.NewWriter(nil)
	writer.WriteByte(byte(len(timestamps)))

	for _, timestamp := range timestamps {
		writer.WriteUint64(timestamp)
	}

	err = l.kv.Put(keyCriticalTimestampHistory[:], writer.Bytes())
	if err != nil {
		return errors.Wrap(err, "wavelet: failed to save critical timestamp history")
	}

	return nil
}

func (l *Ledger) RegisterProcessor(tag byte, processor TransactionProcessor) {
	l.processors[tag] = processor
}

// collapseTransactions takes all transactions recorded in the graph view so far, and
// applies all valid ones to a snapshot of all accounts stored in the ledger.
//
// It returns an updated accounts snapshot after applying all finalized transactions.
func (l *Ledger) collapseTransactions() accounts {
	snapshot := l.snapshotAccounts()

	visited := make(map[[blake2b.Size256]byte]struct{})
	queue := queue.New()

	queue.PushBack(l.view.root)

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		for _, childrenID := range popped.children {
			if _, seen := visited[childrenID]; !seen {
				queue.PushBack(l.view.transactions[childrenID])
			}
		}

		visited[popped.ID] = struct{}{}

		// If any errors occur while applying our transaction to our accounts
		// snapshot, silently log it and continue applying other transactions.
		if err := l.applyTransactionToSnapshot(snapshot, popped); err != nil {
			log.Warn().Err(err).Msg("Got an error while collapsing down transactions.")
		}
	}

	return snapshot
}

func (l *Ledger) applyTransactionToSnapshot(ss accounts, tx *Transaction) error {
	if !ss.snapshot {
		return errors.New("wavelet: to keep things safe, pass in an accounts instance that is a snapshot")
	}

	processor, exists := l.processors[tx.Tag]
	if !exists {
		return errors.Errorf("wavelet: transaction processor not registered for tag %d", tx.Tag)
	}

	ctx := newTransactionContext(ss, tx)

	err := ctx.apply(processor)
	if err != nil {
		return errors.Wrap(err, "wavelet: could not apply transaction")
	}

	return nil
}

func (l *Ledger) HasTransactionInView(id [blake2b.Size256]byte) bool {
	_, exists := l.view.transactions[id]
	return exists
}

func (l *Ledger) ViewID() uint64 {
	return l.viewID.Load()
}

func (l *Ledger) Difficulty() uint64 {
	return l.difficulty.Load()
}
