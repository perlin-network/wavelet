package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/perlin-network/noise/identity"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"math"
	"sort"
	"sync"
	"time"
)

type Ledger struct {
	Accounts accounts

	kv store.KV

	resolver *Snowball
	view     *graph

	processors map[byte]TransactionProcessor

	awaiting map[common.TransactionID][]common.TransactionID
	buffered map[common.TransactionID]*Transaction

	bufferMu sync.Mutex
	resetMu  sync.Mutex

	cacheCollapsible *lru
}

func NewLedger(kv store.KV, genesisPath string) *Ledger {
	ledger := &Ledger{
		Accounts: newAccounts(kv),

		kv: kv,

		resolver: NewSnowball().
			WithK(sys.SnowballQueryK).
			WithAlpha(sys.SnowballQueryAlpha).
			WithBeta(sys.SnowballQueryBeta),

		processors: make(map[byte]TransactionProcessor),

		awaiting: make(map[common.TransactionID][]common.TransactionID),
		buffered: make(map[common.TransactionID]*Transaction),

		cacheCollapsible: newLRU(128),
	}

	buf, err := kv.Get(keyLedgerGenesis[:])

	// If the database has existed before, load all details of the ledger.
	if len(buf) != 0 && err == nil {
		ledger.view = newGraph(kv, nil)
		return ledger
	}

	ledger.saveViewID(0)
	ledger.saveDifficulty(uint64(sys.MinDifficulty))

	genesis, err := performInception(ledger.Accounts, genesisPath)
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

	tx.ParentIDs = l.view.findEligibleParents(l.ViewID())
	if len(tx.ParentIDs) == 0 {
		return errors.New("wavelet: no eligible parents currently available, please try again")
	}

	// Lexicographically sort parent IDs.
	sort.Slice(tx.ParentIDs, func(i, j int) bool {
		return bytes.Compare(tx.ParentIDs[i][:], tx.ParentIDs[j][:]) < 0
	})

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
	/** BASIC ASSERTIONS **/

	// If the transaction is our root transaction, assume that we have already voted positively for it.
	if tx.ID == l.Root().ID {
		return errors.Wrap(VoteAccepted, "tx is our view-graphs root")
	}

	// Assert that the transactions content is valid.
	if err := AssertValidTransaction(tx); err != nil {
		return errors.Wrap(VoteRejected, err.Error())
	}

	// Assert that the transaction was made within our current view ID.
	if err := l.AssertInView(tx); err != nil {
		return errors.Wrap(VoteRejected, err.Error())
	}

	critical := tx.IsCritical(l.Difficulty())

	if critical {
		// If our node already prefers a critical transaction, reject the incoming transaction.
		//if preferred := l.resolver.Preferred(); preferred != nil && tx.ID != preferred.ID {
		//	return errors.Wrapf(VoteRejected, "prefer other critical transaction %x", preferred.ID)
		//}

		// Assert that the critical transaction has a valid timestamp history.
		if err := AssertValidCriticalTimestamps(tx); err != nil {
			return errors.Wrap(VoteRejected, err.Error())
		}
	}

	/** PARENT ASSERTIONS **/

	// Assert that we have all of the transactions parents in our view-graph.
	if err := l.bufferIfMissingAncestors(tx); err != nil {
		return errors.Wrap(VoteRejected, err.Error())
	}

	// Assert that the transaction has a sane timestamp with respect to its parents.
	if err := AssertValidTimestamp(l.view, tx); err != nil {
		return errors.Wrap(VoteRejected, err.Error())
	}

	// Assert that the transactions parents are at an eligible graph depth.
	if err := AssertValidParentDepths(l.view, tx); err != nil {
		return errors.Wrap(VoteRejected, err.Error())
	}

	// Assert that we have the entire transactions ancestry, which is needed
	// to collapse down the transaction.
	if critical {
		if err := l.AssertCollapsible(tx); err != nil {
			return errors.Wrap(VoteRejected, err.Error())
		}
	}

	// Add the transaction to our view-graph if we have its parents in-store.
	if err := l.view.addTransaction(tx); err != nil {
		switch errors.Cause(err) {
		case ErrTxAlreadyExists:
			return errors.Wrap(VoteAccepted, "tx already accepted beforehand")
		default:
			return errors.Wrap(VoteRejected, err.Error())
		}
	}

	// If our node does not prefer any critical transaction yet, set a critical
	// transaction to initially prefer.
	if critical {
		if preferred := l.resolver.Preferred(); preferred == nil && tx.ID != l.Root().ID {
			l.resolver.Prefer(tx)
		}
	}

	l.revisitBufferedTransactions(tx)

	return VoteAccepted
}

// bufferIfMissingAncestors asserts that we have all of the transactions ancestors
// in our view-graph.
//
// If not, we buffer the transaction such that some day, once we have its
// parents, we will attempt to re-add the transaction to the view-graph.
func (l *Ledger) bufferIfMissingAncestors(tx *Transaction) error {
	visited := make(map[common.TransactionID]struct{})
	visited[l.Root().ID] = struct{}{}

	q := queue.New()

	var missing []common.TransactionID

	for _, parentID := range tx.ParentIDs {
		if parent, exists := l.view.lookupTransaction(parentID); exists {
			q.PushBack(parent)
		} else {
			missing = append(missing, parentID)
		}

		visited[parentID] = struct{}{}
	}

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

		if _, exists := l.view.lookupTransaction(popped.ID); !exists {
			missing = append(missing, popped.ID)
		}
	}

	if len(missing) > 0 {
		l.bufferMu.Lock()
		for _, missingID := range missing {
			l.awaiting[missingID] = append(l.awaiting[missingID], tx.ID)
		}
		l.buffered[tx.ID] = tx
		l.bufferMu.Unlock()

		return ErrParentsNotAvailable
	}

	return nil
}

// revisitBufferedTransactions checks that once a specified transaction is added
// to the view-graph, if any buffered transactions may then be eligible to be
// provided another attempt at being added to the view-graph.
func (l *Ledger) revisitBufferedTransactions(tx *Transaction) {
	l.bufferMu.Lock()
	buffer, exists := l.awaiting[tx.ID]
	l.bufferMu.Unlock()

	if !exists {
		return
	}

	unbuffered := make(map[common.TransactionID]struct{})

	for _, id := range buffer {
		l.bufferMu.Lock()
		txx, exists := l.buffered[id]
		l.bufferMu.Unlock()

		if !exists {
			continue
		}

		if _, unbuffered := unbuffered[txx.ID]; unbuffered {
			continue
		}

		if err := l.ReceiveTransaction(txx); errors.Cause(err) == VoteAccepted {
			unbuffered[txx.ID] = struct{}{}
		}
	}

	l.bufferMu.Lock()
	for id := range unbuffered {
		delete(l.buffered, id)
	}
	delete(l.awaiting, tx.ID)
	l.bufferMu.Unlock()
}

// AssertInView asserts if the transaction was proposed and broadcasted
// at the present view ID our ledger is at.
func (l *Ledger) AssertInView(tx *Transaction) error {
	if tx.ViewID != l.ViewID() {
		return errors.Errorf("tx has view ID %d, but current view ID is %d", tx.ViewID, l.ViewID())
	}

	return nil
}

// AssertCollapsible asserts if the transactions ancestry, once collapsed, yields a
// Merkle root hash which is equivalent to the provided transactions Merkle root
// hash.
func (l *Ledger) AssertCollapsible(tx *Transaction) error {
	if collapsible, hit := l.cacheCollapsible.load(tx.ID); hit && collapsible.(bool) {
		return nil
	}

	// If the transaction is critical, assert the transaction has a valid accounts merkle root.
	snapshot := l.collapseTransactions(tx.ParentIDs, false)

	if snapshot.tree.Checksum() != tx.AccountsMerkleRoot {
		return errors.Errorf("tx is critical but has invalid accounts root "+
			"checksum; collapsing down the critical transactions parents gives %x as the root, "+
			"but the tx has %x as a root", snapshot.tree.Checksum(), tx.AccountsMerkleRoot)
	}

	l.cacheCollapsible.put(tx.ID, true)
	return nil
}

func (l *Ledger) Reset(newRoot *Transaction, newState accounts) error {
	l.resetMu.Lock()
	defer l.resetMu.Unlock()

	// Reset any conflict resolving-related data.
	l.resolver.Reset()

	// Increment the view ID by 1.
	l.saveViewID(newRoot.ViewID + 1)

	l.Accounts = newState

	// Commit all account state to the database.
	if err := l.Accounts.Commit(); err != nil {
		return errors.Wrap(err, "wavelet: failed to collapse and commit new ledger state to db")
	}

	// Reset the view-graph with the new root.
	l.view.reset(newRoot)

	l.bufferMu.Lock()
	l.awaiting = make(map[common.TransactionID][]common.TransactionID)
	l.buffered = make(map[common.TransactionID]*Transaction)
	l.bufferMu.Unlock()

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

func (l *Ledger) ProcessQuery(counts map[common.TransactionID]float64, transactions map[common.TransactionID]*Transaction) (bool, error) {
	// If there are zero preferred critical transactions from other nodes, return nil.
	if len(counts) == 0 {
		return false, nil
	}

	l.resolver.Tick(counts, transactions)

	// If a consensus has been decided on the next critical transaction, then reset
	// the view-graph, increment the current view ID, and update the current ledgers
	// difficulty.
	if l.resolver.Decided() {
		root := l.resolver.Preferred()
		old := l.Root()

		if err := l.Reset(root, l.collapseTransactions(root.ParentIDs, true)); err != nil {
			return false, errors.Wrap(err, "wavelet: failed to reset ledger to advance to new view ID")
		}

		logger := log.Consensus("round_end")
		logger.Info().
			Uint64("old_view_id", old.ViewID+1).
			Uint64("new_view_id", root.ViewID+1).
			Hex("new_root", root.ID[:]).
			Hex("old_root", old.ID[:]).
			Hex("new_accounts_checksum", root.AccountsMerkleRoot[:]).
			Hex("old_accounts_checksum", old.AccountsMerkleRoot[:]).
			Msg("Finalized consensus round, and incremented view ID.")

		return true, nil
	}

	return false, nil
}

func (l *Ledger) RegisterProcessor(tag byte, processor TransactionProcessor) {
	l.processors[tag] = processor
}

// collapseTransactions takes all transactions recorded in the graph view so far, and
// applies all valid ones to a snapshot of all accounts stored in the ledger.
//
// It returns an updated accounts snapshot after applying all finalized transactions.
func (l *Ledger) collapseTransactions(parentIDs []common.AccountID, logging bool) accounts {
	ss := l.Accounts.SnapshotAccounts()
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
		popped := applyQueue.PopBack().(*Transaction)

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
	if len(candidates) == 0 || len(candidates) != len(stakes) || totalStake == 0 {
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

func (l *Ledger) Transactions(offset, limit uint64, sender, creator common.AccountID) (transactions []*Transaction) {
	l.view.Lock()

	for _, tx := range l.view.transactions {
		if (sender == common.ZeroAccountID && creator == common.ZeroAccountID) || (sender != common.ZeroAccountID && tx.Sender == sender) || (creator != common.ZeroAccountID && tx.Creator == creator) {
			transactions = append(transactions, tx)
		}
	}

	l.view.Unlock()

	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].depth < transactions[j].depth
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

func (l *Ledger) Resolver() *Snowball {
	return l.resolver
}

func (l *Ledger) QueryMissingTransactions() ([]common.TransactionID, bool) {
	l.bufferMu.Lock()
	defer l.bufferMu.Unlock()

	var ids []common.TransactionID

	for id := range l.awaiting {
		ids = append(ids, id)

		// Only request at most 255 awaiting transactions.
		if len(ids) == math.MaxUint8 {
			break
		}
	}

	if len(ids) > 0 {
		return ids, true
	}

	return nil, false
}
