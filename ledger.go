package wavelet

import (
	"encoding/binary"
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
	"sync"
	"time"
)

var (
	ErrTxNotCritical = errors.New("wavelet: tx is not critical")
)

type Ledger struct {
	accounts

	kv store.KV

	resolver conflict.Resolver
	view     *graph

	processors map[byte]TransactionProcessor

	viewID     atomic.Uint64
	difficulty atomic.Uint64

	genesis *Transaction

	mu sync.Mutex
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

	ledger.difficulty.Store(uint64(sys.MinDifficulty))

	genesis, err := performInception(ledger.accounts, genesisPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to perform inception with genesis data from %q.", genesisPath)
	}

	// Instantiate the view-graph for our ledger.
	ledger.genesis = genesis
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
	if len(tx.ParentIDs) == 0 {
		return errors.New("wavelet: no eligible parents currently available, please try again")
	}

	tx.Timestamp = uint64(time.Duration(time.Now().UnixNano()) / time.Millisecond)

	if tx.IsCritical(l.Difficulty()) {
		snapshot := l.collapseTransactions(tx)
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
	l.mu.Lock()
	defer l.mu.Unlock()

	if tx.ID == l.view.Root().ID {
		return VoteAccepted
	}

	var zero [blake2b.Size256]byte

	critical := tx.IsCritical(l.Difficulty())
	preferred := l.resolver.Preferred()

	// If our node already prefers a critical transaction, reject the
	// incoming transaction.
	if critical && preferred != zero && tx.ID != preferred {
		return errors.Wrap(VoteRejected, "wavelet: prefer other critical transaction")
	}

	if !l.assertValidTimestamp(tx) {
		return errors.Wrap(VoteRejected, "wavelet: either tx timestamp is out of bounds, or parents not available")
	}

	if !l.assertValidParentDepths(tx) {
		return errors.Wrap(VoteRejected, "wavelet: either parent depths are out of bounds, or parents not available")
	}

	// Reject transaction if the parents are not available.
	switch errors.Cause(l.view.addTransaction(tx)) {
	case ErrParentsNotAvailable:
		return errors.Wrap(VoteRejected, "wavelet: parents for transaction are not in our view-graph")
	case ErrTxAlreadyExists:
	}

	// If our node does not prefer any critical transaction yet, set a critical
	// transaction to initially prefer.
	if critical && preferred == zero {
		l.resolver.Prefer(tx.ID)
	}

	return VoteAccepted
}

func (l *Ledger) assertValidParentDepths(tx *Transaction) bool {
	for _, parentID := range tx.ParentIDs {
		parent, stored := l.view.lookupTransaction(parentID)

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
	q := queue.New()

	for _, parentID := range tx.ParentIDs {
		if parent, stored := l.view.lookupTransaction(parentID); stored {
			q.PushBack(parent)
		} else {
			return false
		}

		visited[parentID] = struct{}{}
	}

	var timestamps []uint64

	for q.Len() > 0 {
		popped := q.PopFront().(*Transaction)

		timestamps = append(timestamps, popped.Timestamp)

		if popped == l.view.Root() || len(timestamps) == sys.MedianTimestampNumAncestors {
			break
		}

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				parent, stored := l.view.lookupTransaction(parentID)

				if !stored {
					return false
				}

				q.PushBack(parent)
				visited[popped.ID] = struct{}{}
			}
		}
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

func (l *Ledger) ProcessQuery(tx *Transaction, responses map[[blake2b.Size256]byte]bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !tx.IsCritical(l.Difficulty()) {
		return ErrTxNotCritical
	}

	if len(responses) == 0 {
		return errors.New("wavelet: got no query responses for critical transaction")
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

	if maxStake == 0 {
		return errors.New("wavelet: all nodes rejected critical transaction; couldn't compute max stake")
	}

	for _, stake := range stakes {
		votes = append(votes, float64(stake)/float64(maxStake)/float64(len(responses)))
	}

	//log.Debug().Floats64("weighed_votes", votes).Msg("Weighed votes with stakes, and updated the consensus mechanism.")

	// Update conflict resolver.
	l.resolver.Tick(tx.ID, votes)

	// If a consensus has been decided on the next critical transaction, then reset
	// the view-graph, increment the current view ID, and update the current ledgers
	// difficulty.
	if l.resolver.Decided() {
		old := l.view.Root()

		rootID := l.resolver.Preferred()
		root, recorded := l.view.lookupTransaction(rootID)

		if !recorded {
			return errors.New("wavelet: could not find newly critical tx in view graph")
		}

		l.resolver.Reset()

		viewID := l.viewID.Add(1)

		ss := l.collapseTransactions(root)
		ss.tree.SetViewID(viewID)
		ss.snapshot = false

		l.accounts = ss

		err := l.CommitAccounts()
		if err != nil {
			return errors.Wrap(err, "wavelet: failed to collapse and commit new ledger state to db")
		}

		l.view.reset(root)

		err = l.adjustDifficulty(root)
		if err != nil {
			return errors.Wrap(err, "wavelet: failed to adjust difficulty")
		}

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
	} else {
		// Assume the genesis timestamp is perfect.
		timestamps = append(timestamps, critical.Timestamp-sys.ExpectedConsensusTimeMilliseconds)
	}

	timestamps = append(timestamps, critical.Timestamp)

	// Prune away critical timestamp history if needed.
	if len(timestamps) > sys.CriticalTimestampAverageWindowSize+1 {
		timestamps = timestamps[len(timestamps)-sys.CriticalTimestampAverageWindowSize+1:]
	}

	var deltas []uint64

	// Compute first-order differences across timestamps.
	for i := 1; i < len(timestamps); i++ {
		deltas = append(deltas, timestamps[i]-timestamps[i-1])
	}

	mean := computeMeanTimestamp(deltas)

	// Adjust the current ledgers difficulty to:
	//
	// DIFFICULTY = min(MAX_DIFFICULTY,
	// 	max(MIN_DIFFICULTY,
	// 		(DIFFICULTY - MAX_DIFFICULTY_DELTA / 2) *
	// 			(MAX_DIFFICULTY_DELTA / 2 * max(2,
	// 				10 seconds / average(time it took to accept critical transaction for the last 5 critical transactions)
	// 			)
	// 		)
	// 	)
	expectedTimeFactor := float64(sys.ExpectedConsensusTimeMilliseconds) / float64(mean)
	if expectedTimeFactor > 2.0 {
		expectedTimeFactor = 2.0
	}

	adjusted := int(l.Difficulty()) - int(sys.MaxDifficultyDelta/2) + int(float64(sys.MaxDifficultyDelta)/2.0*expectedTimeFactor)
	if adjusted < sys.MinDifficulty {
		adjusted = sys.MinDifficulty
	}

	original := l.difficulty.Swap(uint64(adjusted))

	log.Info().
		Uint64("old_difficulty", original).
		Int("new_difficulty", adjusted).
		Msg("Ledger difficulty has been adjusted.")

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
func (l *Ledger) collapseTransactions(critical *Transaction) accounts {
	snapshot := l.snapshotAccounts()

	visited := make(map[[blake2b.Size256]byte]struct{})
	visited[l.view.Root().ID] = struct{}{}

	q := queue.New()

	for _, parentID := range critical.ParentIDs {
		if parent, exists := l.view.lookupTransaction(parentID); exists {
			q.PushBack(parent)
		}

		visited[parentID] = struct{}{}
	}

	applyQueue := queue.New()

	for q.Len() > 0 {
		popped := q.PopFront().(*Transaction)

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				if parent, exists := l.view.lookupTransaction(parentID); exists {
					q.PushBack(parent)
				}

				visited[parentID] = struct{}{}
			}
		}

		applyQueue.PushBack(popped)
	}

	// Apply transactions in reverse order from the root of the view-graph all
	// the way up to the newly created critical transaction.
	for applyQueue.Len() > 0 {
		popped := applyQueue.PopFront().(*Transaction)

		// If any errors occur while applying our transaction to our accounts
		// snapshot, silently log it and continue applying other transactions.
		if err := l.applyTransactionToSnapshot(snapshot, popped); err != nil {
			log.Warn().Err(err).Msg("Got an error while collapsing down transactions.")
		}

		if err := l.rewardValidators(snapshot, popped); err != nil {
			log.Warn().Err(err).Msg("Failed to reward a validator while collapsing down transactions.")
		}
	}

	return snapshot
}

func (l *Ledger) applyTransactionToSnapshot(ss accounts, tx *Transaction) error {
	if !ss.snapshot {
		return errors.New("wavelet: to keep things safe, pass in an accounts instance that is a snapshot")
	}

	ctx := newTransactionContext(ss, tx)

	err := ctx.apply(l.processors)
	if err != nil {
		return errors.Wrap(err, "wavelet: could not apply transaction to snapshot")
	}

	return nil
}

func (l *Ledger) rewardValidators(ss accounts, tx *Transaction) error {
	var candidates []*Transaction
	var stakes []uint64
	var totalStake uint64

	visited := make(map[[PublicKeySize]byte]struct{})
	q := queue.New()

	for _, parentID := range tx.ParentIDs {
		if parent, exists := l.view.lookupTransaction(parentID); exists {
			q.PushBack(parent)
		}

		visited[parentID] = struct{}{}
	}

	// Ignore error; should be impossible as not using HMAC mode.
	hasher, _ := blake2b.New256(nil)

	for q.Len() > 0 {
		popped := q.PopFront().(*Transaction)

		// If we exceed the max eligible depth we search for candidate
		// validators to reward from, stop traversing.
		if popped.depth+sys.MaxEligibleParentsDepthDiff < tx.depth {
			continue
		}

		// Filter for all ancestral transactions not from the same sender,
		// and within the desired graph depth.
		if popped.Sender != tx.Sender {
			stake, _ := ss.ReadAccountStake(popped.Sender)

			candidates = append(candidates, popped)
			stakes = append(stakes, stake)

			totalStake += stake

			// Record entropy source.
			_, err := hasher.Write(popped.ID[:])
			if err != nil {
				return errors.Wrap(err, "stake: failed to hash transaction ID for entropy src")
			}
		}

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				if parent, exists := l.view.lookupTransaction(parentID); exists {
					q.PushBack(parent)
				}

				visited[parentID] = struct{}{}
			}
		}
	}

	// If there are no eligible rewardee candidates, do not reward anyone.
	if len(candidates) == 0 || totalStake == 0 {
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

	senderBalance, _ := ss.ReadAccountBalance(tx.Sender)
	recipientBalance, _ := ss.ReadAccountBalance(rewardee.Sender)

	deducted := sys.ValidatorRewardAmount

	if senderBalance < deducted {
		return errors.Errorf("stake: sender does not have enough PERLs to pay transaction fees (requested %d PERLs)", deducted)
	}

	ss.WriteAccountBalance(tx.Sender, senderBalance-deducted)
	ss.WriteAccountBalance(rewardee.Sender, recipientBalance+deducted)

	return nil
}

func (l *Ledger) FindTransaction(id [blake2b.Size256]byte) *Transaction {
	tx, _ := l.view.lookupTransaction(id)
	return tx
}

func (l *Ledger) HasTransactionInView(id [blake2b.Size256]byte) bool {
	_, exists := l.view.lookupTransaction(id)
	return exists
}

func (l *Ledger) Transactions(offset, limit uint64) []*Transaction {
	return l.view.Transactions(offset, limit)
}

func (l *Ledger) Root() *Transaction {
	return l.view.Root()
}

func (l *Ledger) ViewID() uint64 {
	return l.viewID.Load()
}

func (l *Ledger) Difficulty() uint64 {
	return l.difficulty.Load()
}

func (l *Ledger) Resolver() conflict.Resolver {
	return l.resolver
}
