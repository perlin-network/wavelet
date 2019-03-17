package wavelet

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/heptio/workgroup"
	"github.com/perlin-network/noise/identity"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"runtime"
	"sort"
	"sync"
	"time"
)

type EventBroadcast struct {
	Tag       byte
	Payload   []byte
	Creator   common.AccountID
	Signature common.Signature

	Result chan Transaction
	Error  chan error
}

type EventIncomingGossip struct {
	TX Transaction

	Vote chan error
}

type VoteGossip struct {
	Voter common.AccountID
	Ok    bool
}

type EventGossip struct {
	TX Transaction

	Result chan []VoteGossip
	Error  chan error
}

type EventIncomingQuery struct {
	TX Transaction

	Response chan *Transaction
	Error    chan error
}

type VoteQuery struct {
	Voter     common.AccountID
	Preferred Transaction
}

type EventQuery struct {
	TX Transaction

	Result chan []VoteQuery
	Error  chan error
}

type Ledger struct {
	keys identity.Keypair
	kv   store.KV

	v *graph
	a *accounts
	r *Snowball

	processors map[byte]TransactionProcessor

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

	kill chan struct{}
}

func NewLedger(keys identity.Keypair, kv store.KV) *Ledger {
	broadcastQueue := make(chan EventBroadcast, 128)

	gossipIn := make(chan EventIncomingGossip, 128)
	gossipOut := make(chan EventGossip, 128)

	queryIn := make(chan EventIncomingQuery, 128)
	queryOut := make(chan EventQuery, 128)

	accounts := newAccounts(kv)

	genesis, err := performInception(accounts.tree, nil)
	if err != nil {
		panic(err)
	}

	err = accounts.commit(nil)
	if err != nil {
		panic(err)
	}

	view := newGraph(kv, genesis)

	return &Ledger{
		keys: keys,
		kv:   kv,

		v: view,
		a: accounts,
		r: NewSnowball().WithK(sys.SnowballQueryK).WithAlpha(sys.SnowballQueryAlpha).WithBeta(sys.SnowballQueryBeta),

		processors: map[byte]TransactionProcessor{
			sys.TagNop:      ProcessNopTransaction,
			sys.TagTransfer: ProcessTransferTransaction,
			sys.TagContract: ProcessContractTransaction,
			sys.TagStake:    ProcessStakeTransaction,
		},

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

		kill: make(chan struct{}),
	}
}

/** BEGIN EXPORTED METHODS **/

func (l *Ledger) ViewID() uint64 {
	return l.v.loadViewID()
}

func (l *Ledger) Difficulty() uint64 {
	return l.v.loadDifficulty()
}

func (l *Ledger) Root() *Transaction {
	return l.v.loadRoot()
}

func (l *Ledger) Height() uint64 {
	return l.v.height.Load()
}

func (l *Ledger) Snapshot() *avl.Tree {
	return l.a.snapshot()
}

func (l *Ledger) FindTransaction(id common.TransactionID) (*Transaction, bool) {
	return l.v.lookupTransaction(id)
}

func (l *Ledger) ListTransactions(offset, limit uint64, sender, creator common.AccountID) (transactions []*Transaction) {
	l.v.Lock()

	for _, tx := range l.v.transactions {
		if (sender == common.ZeroAccountID && creator == common.ZeroAccountID) || (sender != common.ZeroAccountID && tx.Sender == sender) || (creator != common.ZeroAccountID && tx.Creator == creator) {
			transactions = append(transactions, tx)
		}
	}

	l.v.Unlock()

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

/** END EXPORTED METHODS **/

func NewTransaction(sender identity.Keypair, tag byte, payload []byte) (Transaction, error) {
	tx := Transaction{Tag: tag, Payload: payload}

	signature, err := eddsa.Sign(sender.PrivateKey(), append([]byte{tx.Tag}, tx.Payload...))
	if err != nil {
		return tx, err
	}

	copy(tx.Creator[:], sender.PublicKey())
	copy(tx.CreatorSignature[:], signature)

	return tx, nil
}

func (l *Ledger) attachSenderToTransaction(tx Transaction) (Transaction, error) {
	copy(tx.Sender[:], l.keys.PublicKey())

	if tx.ParentIDs = l.v.findEligibleParents(); len(tx.ParentIDs) == 0 {
		return tx, errors.New("no eligible parents available, please try again")
	}

	sort.Slice(tx.ParentIDs, func(i, j int) bool {
		return bytes.Compare(tx.ParentIDs[i][:], tx.ParentIDs[j][:]) < 0
	})

	tx.Timestamp = uint64(time.Duration(time.Now().UnixNano()) / time.Millisecond)

	for _, parentID := range tx.ParentIDs {
		if parent, exists := l.v.lookupTransaction(parentID); exists && tx.Timestamp <= parent.Timestamp {
			tx.Timestamp = parent.Timestamp + 1
		}
	}

	tx.ViewID = l.v.loadViewID()

	if tx.IsCritical(l.v.loadDifficulty()) {
		snapshot, err := l.collapseTransactions(tx, false)
		if err != nil {
			return tx, errors.Wrap(err, "unable to collapse ancestry to create critical transaction")
		}

		tx.AccountsMerkleRoot = snapshot.Checksum()

		root := l.v.loadRoot()

		tx.DifficultyTimestamps = append(root.DifficultyTimestamps, root.Timestamp)

		if size := computeCriticalTimestampWindowSize(tx.ViewID); len(tx.DifficultyTimestamps) > size {
			tx.DifficultyTimestamps = tx.DifficultyTimestamps[len(tx.DifficultyTimestamps)-size:]
		}
	}

	senderSignature, err := eddsa.Sign(l.keys.PrivateKey(), tx.Write())
	if err != nil {
		return tx, errors.Wrap(err, "failed to make sender signature")
	}

	copy(tx.SenderSignature[:], senderSignature)

	tx.rehash()

	return tx, nil
}

// collapseTransactions takes all transactions recorded in the graph view so far, and
// applies all valid ones to a snapshot of all accounts stored in the ledger.
//
// It returns an updated accounts snapshot after applying all finalized transactions.
func (l *Ledger) collapseTransactions(tx Transaction, logging bool) (*avl.Tree, error) {
	root := l.v.loadRoot()

	ss := l.a.snapshot()
	ss.SetViewID(root.ViewID + 1)

	visited := make(map[common.TransactionID]struct{})
	visited[root.ID] = struct{}{}

	q := queue.New()

	for _, parentID := range tx.ParentIDs {
		if parent, exists := l.v.lookupTransaction(parentID); exists {
			q.PushBack(parent)
		} else {
			return ss, errors.New("not all ancestry of tx provided are available")
		}

		visited[parentID] = struct{}{}
	}

	applyQueue := queue.New()

	for q.Len() > 0 {
		popped := q.PopFront().(*Transaction)

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				if parent, exists := l.v.lookupTransaction(parentID); exists {
					q.PushBack(parent)
				} else {
					return ss, errors.New("not all ancestry of tx provided are available")
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

	return ss, nil
}

func (l *Ledger) applyTransactionToSnapshot(ss *avl.Tree, tx *Transaction) error {
	ctx := newTransactionContext(ss, tx)

	err := ctx.apply(l.processors)
	if err != nil {
		return errors.Wrap(err, "wavelet: could not apply transaction to snapshot")
	}

	return nil
}

func (l *Ledger) rewardValidators(ss *avl.Tree, tx *Transaction) error {
	var candidates []*Transaction
	var stakes []uint64
	var totalStake uint64

	visited := make(map[common.AccountID]struct{})
	q := queue.New()

	for _, parentID := range tx.ParentIDs {
		if parent, exists := l.v.lookupTransaction(parentID); exists {
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
			stake, _ := ReadAccountStake(ss, popped.Sender)

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
				if parent, exists := l.v.lookupTransaction(parentID); exists {
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

	senderBalance, _ := ReadAccountBalance(ss, tx.Sender)
	recipientBalance, _ := ReadAccountBalance(ss, rewardee.Sender)

	deducted := sys.TransactionFeeAmount

	if senderBalance < deducted {
		return errors.Errorf("stake: sender %x does not have enough PERLs to pay transaction fees (requested %d PERLs) to %x", tx.Sender, deducted, rewardee.Sender)
	}

	WriteAccountBalance(ss, tx.Sender, senderBalance-deducted)
	WriteAccountBalance(ss, rewardee.Sender, recipientBalance+deducted)

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

func (l *Ledger) assertCollapsible(tx Transaction) error {
	snapshot, err := l.collapseTransactions(tx, false)
	if err != nil {
		return err
	}

	if snapshot.Checksum() != tx.AccountsMerkleRoot {
		return errors.Errorf("collapsing down tranasction %x's ancestry gives an accounts checksum of %x, but the transaction has %x recorded as an accounts checksum instead", tx.ID, snapshot.Checksum(), tx.AccountsMerkleRoot)
	}

	return nil
}

func Run(l *Ledger) {
	initial := gossiping(l)

	for state := initial; state != nil; {
		state = state(l)
	}
}

type transition func(*Ledger) transition

func gossiping(l *Ledger) transition {
	fmt.Println("NOW GOSSIPING")
	var g workgroup.Group

	for i := 0; i < runtime.NumCPU(); i++ {
		g.Add(continuously(gossip(l)))
		g.Add(continuously(listenForGossip(l)))
	}

	if err := g.Run(); err != nil {
		switch errors.Cause(err) {
		case ErrPreferredSelected:
			return querying
		default:
			fmt.Println(err)
		}
	}

	return nil
}

type stateQuerying struct {
	resetOnce sync.Once
}

func querying(l *Ledger) transition {
	fmt.Println("NOW QUERYING")

	state := new(stateQuerying)

	var g workgroup.Group

	for i := 0; i < runtime.NumCPU(); i++ {
		g.Add(continuously(query(l, state)))
		g.Add(continuously(listenForQueries(l)))
	}

	if err := g.Run(); err != nil {
		switch errors.Cause(err) {
		case ErrConsensusRoundFinished:
			num := len(l.QueryOut)

			for i := 0; i < num; i++ {
				<-l.QueryOut
			}

			return gossiping
		default:
			fmt.Println(err)
		}
	}

	return nil
}

func continuously(fn func(stop <-chan struct{}) error) func(stop <-chan struct{}) error {
	return func(stop <-chan struct{}) error {
		for {
			select {
			case <-stop:
				return nil
			default:
			}

			if err := fn(stop); err != nil {
				switch errors.Cause(err) {
				case ErrTimeout:
				case ErrStopped:
					return nil
				default:
					return err
				}
			}
		}
	}
}

var (
	ErrStopped = errors.New("worker stopped")
	ErrTimeout = errors.New("worker timed out")

	ErrPreferredSelected      = errors.New("attempting to finalize consensus round")
	ErrConsensusRoundFinished = errors.New("consensus round finalized")
)

func gossip(l *Ledger) func(stop <-chan struct{}) error {
	var broadcastNops bool

	return func(stop <-chan struct{}) error {
		snapshot := l.a.snapshot()

		var tx Transaction
		var err error

		var Result chan<- Transaction
		var Error chan<- error

		select {
		case <-l.kill:
			return ErrStopped
		case <-stop:
			return ErrStopped
		case item := <-l.broadcastQueue:
			tx = Transaction{
				Tag:              item.Tag,
				Payload:          item.Payload,
				Creator:          item.Creator,
				CreatorSignature: item.Signature,
			}

			Result = item.Result
			Error = item.Error

			defer close(Result)
			defer close(Error)
		default:
			if !broadcastNops {
				time.Sleep(100 * time.Millisecond)
				return nil
			}

			// Check if we have enough money available to create and broadcast a nop transaction.
			var self common.AccountID
			copy(self[:], l.keys.PublicKey())

			if balance, _ := ReadAccountBalance(snapshot, self); balance < sys.TransactionFeeAmount {
				time.Sleep(100 * time.Millisecond)
				return nil
			}

			// Create a nop transaction.
			tx, err = NewTransaction(l.keys, sys.TagNop, nil)

			if err != nil {
				return err
			}
		}

		tx, err = l.attachSenderToTransaction(tx)

		if err != nil {
			if Error != nil {
				Error <- errors.Wrap(err, "failed to sign off transaction")
			}
			return err
		}

		evt := EventGossip{
			TX:     tx,
			Result: make(chan []VoteGossip, 1),
			Error:  make(chan error, 1),
		}

		select {
		case <-l.kill:
			if Error != nil {
				Error <- ErrStopped
			}

			return ErrStopped
		case <-stop:
			if Error != nil {
				Error <- ErrStopped
			}

			return ErrStopped
		case <-time.After(3 * time.Second):
			if Error != nil {
				Error <- errors.Wrap(ErrTimeout, "gossip queue is full")
			}

			return nil
		case l.gossipOut <- evt:
		}

		select {
		case <-l.kill:
			if Error != nil {
				Error <- ErrStopped
			}

			return ErrStopped
		case <-stop:
			if Error != nil {
				Error <- ErrStopped
			}

			return ErrStopped
		case err := <-evt.Error:
			if err != nil {
				if Error != nil {
					Error <- errors.Wrap(err, "got an error gossiping transaction out")
				}
				return nil
			}
		case votes := <-evt.Result:
			if len(votes) == 0 {
				return nil
			}

			voters := make([]common.AccountID, len(votes))

			for i, vote := range votes {
				voters[i] = vote.Voter
			}

			weights := computeStakeDistribution(snapshot, voters, sys.SnowballQueryK)

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

			if err := AssertInView(l.v, tx); err != nil {
				if Error != nil {
					Error <- err
				}

				return nil
			}

			if err := AssertValidTransaction(tx); err != nil {
				if Error != nil {
					Error <- errors.Wrap(err, "transaction is not valid")
				}
				return nil
			}

			/** At this point, the transaction was successfully gossiped out to our peers. **/

			if err := l.v.addTransaction(&tx); err != nil && errors.Cause(err) != ErrTxAlreadyExists {
				if Error != nil {
					Error <- errors.Wrap(err, "got an error adding transaction to view-graph")
				}
				return nil
			}

			/** At this point, the transaction was successfully added to our view-graph. **/

			critical := tx.IsCritical(l.v.loadDifficulty())

			// If the transaction we created is critical, then stop gossiping and
			// start querying to peers our critical transaction.
			if critical && l.r.Preferred() == nil && tx.ID != l.v.loadRoot().ID {
				l.r.Prefer(tx)
			}

			// If we have nothing else to broadcast and we are not broadcasting out
			// nop transactions, then start broadcasting out nop transactions.
			if len(l.broadcastQueue) == 0 && broadcastNops == false {
				broadcastNops = true
			}

			if Result != nil {
				Result <- tx
			}
		case <-time.After(3 * time.Second):
			if Error != nil {
				Error <- errors.Wrap(ErrTimeout, "did not get back a gossip response")
			}
		}

		if l.r.Preferred() != nil {
			return ErrPreferredSelected
		}

		return nil
	}
}

func listenForGossip(l *Ledger) func(stop <-chan struct{}) error {
	return func(stop <-chan struct{}) error {
		select {
		case <-l.kill:
			return ErrStopped
		case <-stop:
			return ErrStopped
		case evt := <-l.queryIn:
			defer close(evt.Response)
			defer close(evt.Error)

			// If we got a query while we were gossiping, check if the queried transaction
			// is valid. If it is, then we prefer it and move on to querying.

			critical := evt.TX.IsCritical(l.v.loadDifficulty())

			if !critical {
				evt.Error <- errors.New("transaction which was queried is not critical")
				return nil
			}

			if err := AssertInView(l.v, evt.TX); err != nil {
				evt.Error <- err
				return nil
			}

			if err := AssertValidTransaction(evt.TX); err != nil {
				evt.Error <- err
				return nil
			}

			if err := AssertValidAncestry(l.v, evt.TX); err != nil {
				evt.Error <- err
				return nil
			}

			if critical {
				if err := l.assertCollapsible(evt.TX); err != nil {
					evt.Error <- err
					return nil
				}
			}

			if err := l.v.addTransaction(&evt.TX); err != nil && errors.Cause(err) != ErrTxAlreadyExists {
				evt.Error <- errors.Wrap(err, "got an error adding queried transaction to view-graph")
				return nil
			}

			// If the transaction we were queried with is critical, then prefer the incoming
			// queried transaction and move on to querying.
			if critical && l.r.Preferred() == nil && evt.TX.ID != l.v.loadRoot().ID {
				l.r.Prefer(evt.TX)
			}

			// Respond to the query with either:
			//
			// a) the queriers transaction indicating that we will now query with their transaction.
			// b) should they be in a prior view ID, the prior consensus rounds root.
			// c) no response indicating that we do not prefer any transaction.

			if root := l.v.loadRoot(); root.ViewID != 0 && evt.TX.ViewID == root.ViewID-1 {
				evt.Response <- root
			} else if preferred := l.r.Preferred(); preferred != nil {
				evt.Response <- preferred
			} else {
				evt.Response <- nil
			}
		case evt := <-l.gossipIn:
			defer close(evt.Vote)

			// Handle some incoming gossip.

			// Sending `nil` to `evt.Vote` is equivalent to voting that the
			// incoming gossiped transaction has been accepted by our node.

			// If we already have the transaction in our view-graph, we tell the gossiper
			// that the transaction has already been well-received by us.
			if _, exists := l.v.lookupTransaction(evt.TX.ID); exists {
				evt.Vote <- nil
				return nil
			}

			if err := AssertInView(l.v, evt.TX); err != nil {
				evt.Vote <- err
				return nil
			}

			if err := AssertValidTransaction(evt.TX); err != nil {
				evt.Vote <- err
				return nil
			}

			if err := AssertValidAncestry(l.v, evt.TX); err != nil {
				evt.Vote <- err
				return nil
			}

			critical := evt.TX.IsCritical(l.v.loadDifficulty())

			if critical {
				if err := l.assertCollapsible(evt.TX); err != nil {
					evt.Vote <- err
					return nil
				}
			}

			if err := l.v.addTransaction(&evt.TX); err != nil && errors.Cause(err) != ErrTxAlreadyExists {
				evt.Vote <- errors.Wrap(err, "got an error adding transaction to view-graph")
				return nil
			}

			/** At this point, the incoming transaction was successfully added to our view-graph. **/

			// If the transaction we received is critical, then stop gossiping and start querying
			// to peers the provided critical transaction.
			if critical && l.r.Preferred() == nil && evt.TX.ID != l.v.loadRoot().ID {
				l.r.Prefer(evt.TX)
			}

			evt.Vote <- nil
		}

		if l.r.Preferred() != nil {
			return ErrPreferredSelected
		}

		return nil
	}
}

func query(l *Ledger, state *stateQuerying) func(stop <-chan struct{}) error {
	return func(stop <-chan struct{}) error {
		snapshot := l.a.snapshot()
		preferred := l.r.Preferred()

		if preferred == nil {
			return ErrConsensusRoundFinished
		}

		evt := EventQuery{
			TX:     *preferred,
			Result: make(chan []VoteQuery, 1),
			Error:  make(chan error, 1),
		}

		select {
		case <-l.kill:
			return ErrStopped
		case <-stop:
			return ErrStopped
		case <-time.After(3 * time.Second):
			return errors.Wrap(ErrTimeout, "query queue is full")
		case l.queryOut <- evt:
		}

		select {
		case <-l.kill:
			return ErrStopped
		case <-stop:
			return ErrStopped
		case err := <-evt.Error:
			return errors.Wrap(err, "error while querying")
		case votes := <-evt.Result:
			if len(votes) == 0 {
				return nil
			}

			ourViewID := l.v.loadViewID()

			voters := make([]common.AccountID, len(votes))
			counts := make(map[common.TransactionID]float64)
			transactions := make(map[common.TransactionID]Transaction)

			for i, vote := range votes {
				if vote.Preferred.ViewID == ourViewID && vote.Preferred.ID != common.ZeroTransactionID {
					transactions[vote.Preferred.ID] = vote.Preferred
					voters[i] = vote.Voter
				}
			}

			weights := computeStakeDistribution(snapshot, voters, sys.SnowballQueryK)

			for _, vote := range votes {
				if vote.Preferred.ViewID == ourViewID && vote.Preferred.ID != common.ZeroTransactionID {
					counts[vote.Preferred.ID] += weights[vote.Voter]
				}
			}

			l.r.Tick(counts, transactions)

			// Once Snowball has finalized, collapse down our transactions, reset everything, and
			// commit the newly officiated ledger state to our database.

			if l.r.Decided() {
				var exception error

				state.resetOnce.Do(func() {
					newRoot := l.r.Preferred()
					oldRoot := l.v.loadRoot()

					state, err := l.collapseTransactions(*newRoot, true)
					if err != nil {
						exception = errors.Wrap(err, "decided a new root, but got an error collapsing down its ancestry")
						return
					}

					if err = l.a.commit(state); err != nil {
						exception = errors.Wrap(err, "failed to commit collapsed state to our database")
						return
					}

					l.r.Reset()
					l.v.reset(newRoot)

					logger := log.Consensus("round_end")
					logger.Info().
						Uint64("old_view_id", oldRoot.ViewID+1).
						Uint64("new_view_id", newRoot.ViewID+1).
						Hex("new_root", newRoot.ID[:]).
						Hex("old_root", oldRoot.ID[:]).
						Hex("new_accounts_checksum", newRoot.AccountsMerkleRoot[:]).
						Hex("old_accounts_checksum", oldRoot.AccountsMerkleRoot[:]).
						Msg("Finalized consensus round, and incremented view ID.")
				})

				if exception != nil {
					return exception
				}

				return ErrConsensusRoundFinished
			}
		case <-time.After(3 * time.Second):
			return errors.Wrap(ErrTimeout, "did not get back a query response")
		}

		return nil
	}
}

func listenForQueries(l *Ledger) func(stop <-chan struct{}) error {
	return func(stop <-chan struct{}) error {
		select {
		case <-l.kill:
			return ErrStopped
		case <-stop:
			return ErrStopped
		case evt := <-l.queryIn:
			defer close(evt.Response)
			defer close(evt.Error)

			// Respond to the query with either:
			//
			// a) our own preferred transaction.
			// b) should they be in a prior view ID, the prior consensus rounds root.

			if root := l.v.loadRoot(); root.ViewID != 0 && evt.TX.ViewID == root.ViewID-1 {
				evt.Response <- root
			} else if preferred := l.r.Preferred(); preferred != nil {
				evt.Response <- preferred
			} else {
				evt.Response <- nil
			}
		}

		if l.r.Preferred() == nil {
			return ErrConsensusRoundFinished
		}

		return nil
	}
}
