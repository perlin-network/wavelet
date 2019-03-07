package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/noise/identity"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/conflict"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"time"
)

type Ledger struct {
	accounts

	kv store.KV

	resolver conflict.Resolver
	view     *graph

	processors map[byte]TransactionProcessor
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
		// Insert account state Merkle tree root.
		snapshot := l.collapseTransactions(tx.ParentIDs, false)
		tx.AccountsMerkleRoot = snapshot.tree.Checksum()

		// Insert difficulty timestamps.
		tx.DifficultyTimestamps = make([]uint64, len(l.Root().DifficultyTimestamps))
		copy(tx.DifficultyTimestamps, l.Root().DifficultyTimestamps)

		tx.DifficultyTimestamps = append(tx.DifficultyTimestamps, l.Root().Timestamp)

		if size := computeCriticalTimestampWindowSize(tx.ViewID); len(tx.DifficultyTimestamps) > size {
			tx.DifficultyTimestamps = tx.DifficultyTimestamps[len(tx.DifficultyTimestamps)-size:]
		}
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
	// If the transaction is our root transaction, assume that we have already voted positively for it.
	if tx.ID == l.Root().ID {
		return VoteAccepted
	}

	// Assert the transaction is valid.
	if err := AssertValidTransaction(tx); err != nil {
		return errors.Wrap(VoteRejected, err.Error())
	}

	// Assert the transaction was made within our current view ID.
	if err := l.AssertValidViewID(tx); err != nil {
		return errors.Wrap(VoteRejected, err.Error())
	}

	// Assert that the transaction has a sane timestamp, and that we have the transactions parents in-store.
	if !l.assertValidTimestamp(tx) {
		return errors.Wrap(VoteRejected, "either tx timestamp is out of bounds, or parents not available")
	}

	// Assert that the transaction has sane parents, and that we have the transactions parents in-store.
	if !l.assertValidParentDepths(tx) {
		return errors.Wrap(VoteRejected, "either parent depths are out of bounds, or parents not available")
	}

	preferred := l.resolver.Preferred()

	if tx.IsCritical(l.Difficulty()) {
		// If our node already prefers a critical transaction, reject the
		// incoming transaction.
		if preferred != nil && tx.ID != preferred {
			return errors.Wrap(VoteRejected, "prefer other critical transaction")
		}

		// If our node does not prefer any critical transaction yet, set a critical
		// transaction to initially prefer.
		if preferred == nil && tx.ID != l.Root().ID {
			l.resolver.Prefer(tx.ID)
		}
	}

	// Add the transaction to our view-graph if we have its parents in-store.
	if err := l.view.addTransaction(tx); err != nil {
		switch errors.Cause(err) {
		case ErrParentsNotAvailable:
			return errors.Wrap(VoteRejected, "parents for transaction are not in our view-graph")
		case ErrTxAlreadyExists:
		}
	}

	return VoteAccepted
}

func (l *Ledger) AssertValidViewID(tx *Transaction) error {
	// Assert the transaction was made within our current view ID.
	if tx.ViewID != l.ViewID() {
		return errors.Errorf("tx has view ID %d, but current view ID is %d", tx.ViewID, l.ViewID())
	}

	// If the transaction is critical, assert the transaction has a valid accounts merkle root.
	if tx.IsCritical(l.Difficulty()) {
		snapshot := l.collapseTransactions(tx.ParentIDs, false)

		if snapshot.tree.Checksum() != tx.AccountsMerkleRoot {
			return errors.Wrapf(VoteRejected, "tx is critical but has invalid accounts root "+
				"checksum; collapsing down the critical transactions parents gives %x as the root, "+
				"but the tx has %x as a root", snapshot.tree.Checksum(), tx.AccountsMerkleRoot)
		}
	}

	return nil
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

		if popped.ID == l.Root().ID || len(timestamps) == sys.MedianTimestampNumAncestors {
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
		stake, _ := l.accounts.ReadAccountStake(account)

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

func (l *Ledger) Reset(newRoot *Transaction, newState accounts) error {
	// Reset any conflict resolving-related data.
	l.resolver.Reset()

	// Increment the view ID by 1.
	l.saveViewID(newRoot.ViewID + 1)

	l.accounts = newState

	// Commit all account state to the database.
	if err := l.CommitAccounts(); err != nil {
		return errors.Wrap(err, "wavelet: failed to collapse and commit new ledger state to db")
	}

	// Reset the view-graph with the new root.
	l.view.reset(newRoot)

	// Update ledgers difficulty.
	original, adjusted := l.Difficulty(), computeNextDifficulty(newRoot)

	logger := log.Consensus("update_difficulty")
	logger.Info().
		Uint64("old_difficulty", original).
		Uint64("new_difficulty", adjusted).
		Msg("Ledger difficulty has been adjusted.")

	l.saveDifficulty(adjusted)

	return nil
}

func (l *Ledger) ProcessQuery(counts map[interface{}]float64) error {
	// If there are zero preferred critical transactions from other nodes, return nil.
	if len(counts) == 0 {
		return nil
	}

	l.resolver.Tick(counts)

	// If a consensus has been decided on the next critical transaction, then reset
	// the view-graph, increment the current view ID, and update the current ledgers
	// difficulty.
	if l.resolver.Decided() {
		old := l.Root()

		rootID := l.resolver.Preferred().(common.TransactionID)
		root, recorded := l.view.lookupTransaction(rootID)

		if !recorded {
			return errors.New("wavelet: could not find newly critical tx in view graph")
		}

		if err := l.Reset(root, l.collapseTransactions(root.ParentIDs, true)); err != nil {
			return errors.Wrap(err, "wavelet: failed to reset ledger to advance to new view ID")
		}

		logger := log.Consensus("round_end")
		logger.Info().
			Uint64("old_view_id", old.ViewID).
			Uint64("new_view_id", root.ViewID+1).
			Hex("new_root", root.ID[:]).
			Hex("old_root", old.ID[:]).
			Hex("new_accounts_checksum", root.AccountsMerkleRoot[:]).
			Hex("old_accounts_checksum", old.AccountsMerkleRoot[:]).
			Msg("Finalized consensus round, and incremented view ID.")
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
	ss.SetViewID(l.Root().ViewID + 1)

	visited := make(map[common.TransactionID]struct{})
	visited[l.Root().ID] = struct{}{}

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

			if stake > sys.MinimumStake {
				candidates = append(candidates, popped)
				stakes = append(stakes, stake)

				totalStake += stake

				// Record entropy source.
				_, err := hasher.Write(popped.ID[:])
				if err != nil {
					return errors.Wrap(err, "stake: failed to hash transaction ID for entropy src")
				}
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

	deducted := sys.TransactionFeeAmount

	if senderBalance < deducted {
		return errors.Errorf("stake: sender %x does not have enough PERLs to pay transaction fees (requested %d PERLs) to %x", tx.Sender, deducted, rewardee.Sender)
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
