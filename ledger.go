package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/noise/identity"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/conflict"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"sync"
	"time"
)

type Ledger struct {
	accounts

	kv store.KV

	resolver conflict.Resolver
	view     *graph

	processors map[byte]TransactionProcessor

	mu sync.Mutex

	consensusLock sync.RWMutex
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

	buf, err := kv.Get(keyLedgerGenesis[:])

	// If the database has existed before, load all details of the ledger.
	if len(buf) != 0 && err == nil {
		ledger.view = newGraph(kv, nil)
		return ledger
	}

	ledger.saveViewID(0)
	ledger.saveDifficulty(uint64(sys.MinDifficulty))

	genesis, err := performInception(ledger.accounts, genesisPath)
	if err != nil {
		logger := log.Node()
		logger.Fatal().Err(err).Msgf("Failed to perform inception with genesis data from %q.", genesisPath)
	}

	ledger.view = newGraph(kv, genesis)

	_ = kv.Put(keyLedgerGenesis[:], []byte{0x1})

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

	for _, parentID := range tx.ParentIDs {
		if parent, exists := l.view.lookupTransaction(parentID); exists && tx.Timestamp <= parent.Timestamp {
			tx.Timestamp = parent.Timestamp + 1
		}
	}

	tx.ViewID = l.ViewID()

	if tx.IsCritical(l.Difficulty()) {
		snapshot := l.collapseTransactions(tx.ParentIDs, false)
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

	if tx.ViewID < l.ViewID() {
		return errors.Wrapf(VoteRejected, "wavelet: tx has view ID %d, but current view ID is %d", tx.ViewID, l.ViewID())
	}

	if tx.Sender == common.ZeroAccountID || tx.Creator == common.ZeroAccountID {
		return errors.Wrap(VoteRejected, "wavelet: tx must have sender or creator")
	}

	if len(tx.ParentIDs) == 0 {
		return errors.Wrap(VoteRejected, "wavelet: tx must have parents")
	}

	if tx.Tag != sys.TagNop && len(tx.Payload) == 0 {
		return errors.Wrap(VoteRejected, "wavelet: tx must have payload if not a nop transaction")
	}

	if tx.Tag == sys.TagNop && len(tx.Payload) != 0 {
		return errors.Wrap(VoteRejected, "wavelet: tx must have no payload if is a nop transaction")
	}

	if !l.assertValidSignature(tx) {
		return errors.Wrap(VoteRejected, "wavelet: tx has either invalid creator or sender signatures")
	}

	if !l.assertValidTimestamp(tx) {
		return errors.Wrap(VoteRejected, "wavelet: either tx timestamp is out of bounds, or parents not available")
	}

	if !l.assertValidParentDepths(tx) {
		return errors.Wrap(VoteRejected, "wavelet: either parent depths are out of bounds, or parents not available")
	}

	critical := tx.IsCritical(l.Difficulty())
	preferred := l.resolver.Preferred()

	if critical {
		if valid, root := l.assertValidAccountsChecksum(tx); !valid {
			return errors.Wrapf(VoteRejected, "wavelet: tx is critical but has invalid accounts root "+
				"checksum; collapsing down the critical transactions parents gives %x as the root, "+
				"but the tx has %x as a root", root, tx.AccountsMerkleRoot)
		}

		// If our node already prefers a critical transaction, reject the
		// incoming transaction.
		if preferred != nil && tx.ID != preferred {
			return errors.Wrap(VoteRejected, "wavelet: prefer other critical transaction")
		}

		// If our node does not prefer any critical transaction yet, set a critical
		// transaction to initially prefer.
		if preferred == nil && tx.ID != l.view.Root().ID {
			l.resolver.Prefer(tx.ID)
		}
	}

	if err := l.view.addTransaction(tx); err != nil {
		switch errors.Cause(err) {
		case ErrParentsNotAvailable:
			return errors.Wrap(VoteRejected, "wavelet: parents for transaction are not in our view-graph")
		case ErrTxAlreadyExists:
		}
	}

	return VoteAccepted
}

func (l *Ledger) assertValidSignature(tx *Transaction) bool {
	err := eddsa.Verify(tx.Creator[:], append([]byte{tx.Tag}, tx.Payload...), tx.CreatorSignature[:])
	if err != nil {
		return false
	}

	cpy := *tx
	cpy.SenderSignature = common.ZeroSignature

	err = eddsa.Verify(tx.Sender[:], cpy.Write(), tx.SenderSignature[:])
	if err != nil {
		return false
	}

	return true

}

func (l *Ledger) assertValidAccountsChecksum(tx *Transaction) (bool, [avl.MerkleHashSize]byte) {
	snapshot := l.collapseTransactions(tx.ParentIDs, false)
	return snapshot.tree.Checksum() == tx.AccountsMerkleRoot, snapshot.tree.Checksum()
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
	visited := make(map[common.TransactionID]struct{})
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

		if popped.ID == l.view.Root().ID || len(timestamps) == sys.MedianTimestampNumAncestors {
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

func (l *Ledger) ComputeStakeDistribution(accounts []common.AccountID) map[common.AccountID]float64 {
	stakes := make(map[common.AccountID]uint64)
	var maxStake uint64

	for _, account := range accounts {
		stake, _ := l.ReadAccountStake(account)

		if stake < sys.MinimumStake {
			stake = sys.MinimumStake
		}

		if maxStake < stake {
			maxStake = stake
		}

		stakes[account] = stake
	}

	weights := make(map[common.AccountID]float64)

	for account, stake := range stakes {
		weights[account] = float64(stake) / float64(maxStake) / float64(sys.SnowballK)
	}

	return weights
}

func (l *Ledger) Reset(newRoot *Transaction, newState accounts) (uint64, error) {
	l.resolver.Reset()

	l.saveViewID(newRoot.ViewID + 1)

	l.accounts = newState

	err := l.CommitAccounts()
	if err != nil {
		return newRoot.ViewID + 1, errors.Wrap(err, "wavelet: failed to collapse and commit new ledger state to db")
	}

	l.view.reset(newRoot)

	return newRoot.ViewID + 1, nil
}

func (l *Ledger) ProcessQuery(counts map[interface{}]float64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// If there are zero preferred critical transactions from other nodes, return nil.
	if len(counts) == 0 {
		return nil
	}

	l.resolver.Tick(counts)

	// If a consensus has been decided on the next critical transaction, then reset
	// the view-graph, increment the current view ID, and update the current ledgers
	// difficulty.
	if l.resolver.Decided() {
		old := l.view.Root()

		rootID := l.resolver.Preferred().(common.TransactionID)
		root, recorded := l.view.lookupTransaction(rootID)

		if !recorded {
			return errors.New("wavelet: could not find newly critical tx in view graph")
		}

		viewID, err := l.Reset(root, l.collapseTransactions(root.ParentIDs, true))
		if err != nil {
			return errors.Wrap(err, "wavelet: failed to reset ledger to advance to new view ID")
		}

		err = l.adjustDifficulty(root)
		if err != nil {
			return errors.Wrap(err, "wavelet: failed to adjust difficulty")
		}

		logger := log.Consensus("round_end")
		logger.Info().
			Uint64("old_view_id", viewID-1).
			Uint64("new_view_id", viewID).
			Hex("new_root", root.ID[:]).
			Hex("old_root", old.ID[:]).
			Hex("new_accounts_checksum", root.AccountsMerkleRoot[:]).
			Hex("old_accounts_checksum", old.AccountsMerkleRoot[:]).
			Msg("Finalized consensus round, and incremented view ID.")
	}

	return nil
}

func (l *Ledger) adjustDifficulty(critical *Transaction) error {
	var timestamps []uint64

	// Load critical timestamp history if it exists.
	buf, err := l.kv.Get(keyCriticalTimestampHistory[:])
	if len(buf) > 0 && err == nil {
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

	original := l.Difficulty()

	adjusted := int(original) - int(sys.MaxDifficultyDelta/2) + int(float64(sys.MaxDifficultyDelta)/2.0*expectedTimeFactor)
	if adjusted < sys.MinDifficulty {
		adjusted = sys.MinDifficulty
	}

	l.saveDifficulty(uint64(adjusted))

	logger := log.Consensus("update_difficulty")
	logger.Info().
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
func (l *Ledger) collapseTransactions(parentIDs []common.AccountID, logging bool) accounts {
	ss := l.SnapshotAccounts()
	ss.tree.IncrementViewID()

	visited := make(map[common.TransactionID]struct{})
	visited[l.view.Root().ID] = struct{}{}

	q := queue.New()

	for _, parentID := range parentIDs {
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
		if err := l.applyTransactionToSnapshot(ss, popped); err != nil {
			if logging {
				logger := log.TX(popped.ID, popped.Sender, popped.Creator, popped.ParentIDs, popped.Tag, popped.Payload, "failed")
				logger.Log().Err(err).Msg("Failed to apply transaction to the ledger.")
			}
		} else {
			if logging {
				logger := log.TX(popped.ID, popped.Sender, popped.Creator, popped.ParentIDs, popped.Tag, popped.Payload, "applied")
				logger.Log().Msg("Successfully applied transaction to the ledger.")
			}
		}

		if err := l.rewardValidators(ss, popped); err != nil {
			if logging {
				logger := log.Node()
				logger.Warn().Err(err).Msg("Failed to reward a validator while collapsing down transactions.")
			}
		}
	}

	return ss
}

func (l *Ledger) applyTransactionToSnapshot(ss accounts, tx *Transaction) error {
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

	visited := make(map[common.AccountID]struct{})
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

	logger := log.Stake("reward_validator")
	logger.Log().
		Hex("sender", tx.Sender[:]).
		Hex("recipient", rewardee.Sender[:]).
		Hex("sender_tx_id", tx.ID[:]).
		Hex("rewardee_tx_id", rewardee.ID[:]).
		Hex("entropy", entropy).
		Float64("acc", acc).
		Float64("threshold", threshold).Msg("Rewarded validator.")

	return nil
}

func (l *Ledger) FindTransaction(id common.TransactionID) (*Transaction, bool) {
	tx, exists := l.view.lookupTransaction(id)
	return tx, exists
}

func (l *Ledger) Transactions(offset, limit uint64, sender, creator common.AccountID) []*Transaction {
	return l.view.Transactions(offset, limit, sender, creator)
}

func (l *Ledger) Root() *Transaction {
	return l.view.Root()
}

func (l *Ledger) Height() uint64 {
	return l.view.Height()
}

func (l *Ledger) ViewID() uint64 {
	buf, err := l.kv.Get(keyLedgerViewID[:])
	if len(buf) != 8 || err != nil {
		return 0
	}

	return binary.LittleEndian.Uint64(buf)
}

func (l *Ledger) Difficulty() uint64 {
	buf, err := l.kv.Get(keyLedgerDifficulty[:])
	if len(buf) != 8 || err != nil {
		return uint64(sys.MinDifficulty)
	}

	return binary.LittleEndian.Uint64(buf)
}

func (l *Ledger) saveDifficulty(difficulty uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], difficulty)

	_ = l.kv.Put(keyLedgerDifficulty[:], buf[:])
}

func (l *Ledger) saveViewID(viewID uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], viewID)

	_ = l.kv.Put(keyLedgerViewID[:], buf[:])
}

func (l *Ledger) Resolver() conflict.Resolver {
	return l.resolver
}

func (l *Ledger) ConsensusLock() *sync.RWMutex {
	return &l.consensusLock
}
