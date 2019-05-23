package wavelet

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"math/rand"
	"sync"
	"time"
)

type Ledger struct {
	client  *skademlia.Client
	metrics *Metrics

	accounts *Accounts
	rounds   *Rounds
	graph    *Graph

	gossiper  *Gossiper
	finalizer *Snowball
	syncer    *Snowball

	processors map[byte]TransactionProcessor

	consensus sync.WaitGroup

	sync      chan struct{}
	syncTimer *time.Timer
	syncVotes chan vote

	cacheCollapse *LRU
	cacheChunks   *LRU
}

func NewLedger(client *skademlia.Client) *Ledger {
	kv := store.NewInmem()

	metrics := NewMetrics(context.TODO())
	accounts := NewAccounts(kv)

	rounds, err := NewRounds(kv, sys.PruningLimit)

	var round *Round

	if rounds != nil && err != nil {
		genesis := performInception(accounts.tree, nil)
		if err := accounts.Commit(nil); err != nil {
			panic(err)
		}

		ptr := &genesis

		if _, err := rounds.Save(ptr); err != nil {
			panic(err)
		}

		round = ptr
	} else if rounds != nil {
		round = rounds.Latest()
	}

	if round == nil {
		panic("???: COULD NOT FIND GENESIS, OR STORAGE IS CORRUPTED.")
	}

	graph := NewGraph(WithMetrics(metrics), WithRoot(round.End), VerifySignatures())

	gossiper := NewGossiper(context.TODO(), client, metrics)
	finalizer := NewSnowball(WithBeta(sys.SnowballBeta))
	syncer := NewSnowball(WithBeta(sys.SnowballBeta))

	ledger := &Ledger{
		client:  client,
		metrics: metrics,

		accounts: accounts,
		rounds:   rounds,
		graph:    graph,

		gossiper:  gossiper,
		finalizer: finalizer,
		syncer:    syncer,

		processors: map[byte]TransactionProcessor{
			sys.TagNop:      ProcessNopTransaction,
			sys.TagTransfer: ProcessTransferTransaction,
			sys.TagContract: ProcessContractTransaction,
			sys.TagStake:    ProcessStakeTransaction,
			sys.TagBatch:    ProcessBatchTransaction,
		},

		sync:      make(chan struct{}),
		syncTimer: time.NewTimer(0),
		syncVotes: make(chan vote, sys.SnowballK),

		cacheCollapse: NewLRU(16),
		cacheChunks:   NewLRU(1024), // In total, it will take up 1024 * 4MB.
	}

	go ledger.SyncToLatestRound()
	ledger.PerformConsensus()

	return ledger
}

// AddTransaction adds a transaction to the ledger. If the transaction has
// never been added in the ledgers graph before, it is pushed to the gossip
// mechanism to then be gossiped to this nodes peers. If the transaction is
// invalid or fails any validation checks, an error is returned. No error
// is returned if the transaction has already existed int he ledgers graph
// beforehand.
func (l *Ledger) AddTransaction(tx Transaction) error {
	err := l.graph.AddTransaction(tx)

	if err != nil && errors.Cause(err) != ErrAlreadyExists {
		return err
	}

	if err == nil {
		l.gossiper.Push(tx)
	}

	return nil
}

// Protocol returns an implementation of WaveletServer to handle incoming
// RPC and streams for the ledger. The protocol is agnostic to whatever
// choice of network stack is used with Wavelet, though by default it is
// intended to be used with gRPC and Noise.
func (l *Ledger) Protocol() *Protocol {
	return &Protocol{ledger: l}
}

// Graph returns the directed-acyclic-graph of transactions accompanying
// the ledger.
func (l *Ledger) Graph() *Graph {
	return l.graph
}

// PerformConsensus spawns workers related to performing consensus, such as pulling
// missing transactions and incrementally finalizing intervals of transactions in
// the ledgers graph.
func (l *Ledger) PerformConsensus() {
	go l.PullMissingTransactions()
	go l.FinalizeRounds()
}

// PullMissingTransactions is an infinite loop continually sending RPC requests
// to pull any transactions identified to be missing by the ledger. It periodically
// samples a random peer from the network, and requests the peer for the contents
// of all missing transactions by their respective IDs. When the ledger is in amidst
// synchronizing/teleporting ahead to a new round, the infinite loop will be cleaned
// up. It is intended to call PullMissingTransactions() in a new goroutine.
func (l *Ledger) PullMissingTransactions() {
	l.consensus.Add(1)
	defer l.consensus.Done()

	for {
		missing := l.graph.Missing()

		if len(missing) == 0 {
			select {
			case <-l.sync:
				return
			case <-time.After(1 * time.Second):
			}

			continue
		}

		peers := l.client.AllPeers()

		if len(peers) == 0 {
			select {
			case <-l.sync:
				return
			case <-time.After(1 * time.Second):
			}

			continue
		}

		rand.Shuffle(len(peers), func(i, j int) {
			peers[i], peers[j] = peers[j], peers[i]
		})

		req := &DownloadTxRequest{Ids: make([][]byte, len(missing))}

		for i, id := range missing {
			req.Ids[i] = id[:]
		}

		conn := peers[0]
		client := NewWaveletClient(conn)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		batch, err := client.DownloadTx(ctx, req)
		if err != nil {
			fmt.Println("failed to download missing transactions:", err)
			cancel()
			continue
		}
		cancel()

		count := int64(0)

		for _, buf := range batch.Transactions {
			tx, err := UnmarshalTransaction(bytes.NewReader(buf))

			if err != nil {
				continue
			}

			if err := l.AddTransaction(tx); err != nil && errors.Cause(err) != ErrMissingParents {
				fmt.Printf("error adding downloaded tx to graph [%v]: %+v\n", err, tx)
				continue
			}

			count += int64(tx.LogicalUnits())
		}

		l.metrics.downloadedTX.Mark(count)

		select {
		case <-l.sync:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// FinalizeRounds periodically attempts to find an eligible critical transaction suited for the
// current round. If it finds one, it will then proceed to perform snowball sampling over its
// peers to decide on a single critical transaction that serves as an ending point for the
// current consensus round. The round is finalized, transactions of the finalized round are
// applied to the current ledger state, and the graph is updated to cleanup artifacts from
// the old round.
func (l *Ledger) FinalizeRounds() {
	l.consensus.Add(1)
	defer l.consensus.Done()

FINALIZE_ROUNDS:
	for {
		select {
		case <-l.sync:
			return
		default:
		}

		if _, err := SelectPeers(l.client.ClosestPeers(), sys.SnowballK); err != nil {
			select {
			case <-l.sync:
				return
			case <-time.After(1 * time.Second):
			}

			continue FINALIZE_ROUNDS
		}

		current := l.rounds.Latest()
		currentDifficulty := current.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)

		if preferred := l.finalizer.Preferred(); preferred == nil {
			eligible := l.graph.FindEligibleCritical(currentDifficulty)

			if eligible == nil {
				select {
				case <-l.sync:
					return
				case <-time.After(100 * time.Millisecond):
				}

				continue FINALIZE_ROUNDS
			}

			results, err := l.CollapseTransactions(current.Index+1, current.End, *eligible, false)
			if err != nil {
				fmt.Println(err)
				continue
			}

			candidate := NewRound(current.Index+1, results.snapshot.Checksum(), uint64(results.appliedCount), current.End, *eligible)
			l.finalizer.Prefer(&candidate)

			continue FINALIZE_ROUNDS
		}

		workerChan := make(chan *grpc.ClientConn, 16)

		var workerWG sync.WaitGroup
		workerWG.Add(cap(workerChan))

		voteChan := make(chan vote, sys.SnowballK)
		go CollectVotes(l.accounts, l.finalizer, voteChan, &workerWG)

		req := &QueryRequest{RoundIndex: current.Index + 1}

		for i := 0; i < cap(workerChan); i++ {
			go func() {
				for conn := range workerChan {
					f := func() {
						client := NewWaveletClient(conn)

						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

						p := &peer.Peer{}

						res, err := client.Query(ctx, req, grpc.Peer(p))
						if err != nil {
							cancel()
							return
						}

						cancel()

						l.metrics.queried.Mark(1)

						info := noise.InfoFromPeer(p)
						if info == nil {
							return
						}

						voter, ok := info.Get(skademlia.KeyID).(*skademlia.ID)
						if !ok {
							return
						}

						round, err := UnmarshalRound(bytes.NewReader(res.Round))
						if err != nil {
							voteChan <- vote{voter: voter, preferred: nil}
							return
						}

						if round.ID == ZeroRoundID || round.Start.ID == ZeroTransactionID || round.End.ID == ZeroTransactionID {
							voteChan <- vote{voter: voter, preferred: nil}
							return
						}

						if round.Index != current.Index+1 {
							if round.Index > sys.SyncIfRoundsDifferBy+current.Index {
								select {
								case l.syncVotes <- vote{voter: voter, preferred: &round}:
								default:
								}
							}

							return
						}

						if round.Start.ID != current.End.ID {
							return
						}

						if !round.End.IsCritical(currentDifficulty) {
							return
						}

						results, err := l.CollapseTransactions(round.Index, round.Start, round.End, false)
						if err != nil {
							return
						}

						if uint64(results.appliedCount) != round.Applied {
							return
						}

						if results.snapshot.Checksum() != round.Merkle {
							return
						}

						voteChan <- vote{voter: voter, preferred: &round}
					}

					l.metrics.queryLatency.Time(f)
				}

				workerWG.Done()
			}()
		}

		for !l.finalizer.Decided() {
			select {
			case <-l.sync:
				close(workerChan)
				workerWG.Wait()
				workerWG.Add(1)
				close(voteChan)
				workerWG.Wait() // Wait for vote processor worker to close.

				return
			default:
			}

			// Randomly sample a peer to query. If no peers are available, stop querying.

			peers, err := SelectPeers(l.client.ClosestPeers(), 1)
			if err != nil {
				close(workerChan)
				workerWG.Wait()
				workerWG.Add(1)
				close(voteChan)
				workerWG.Wait() // Wait for vote processor worker to close.

				continue FINALIZE_ROUNDS
			}

			conn := peers[0]
			workerChan <- conn
		}

		close(workerChan)
		workerWG.Wait() // Wait for query workers to close.
		workerWG.Add(1)
		close(voteChan)
		workerWG.Wait() // Wait for vote processor worker to close.

		finalized := l.finalizer.Preferred()
		l.finalizer.Reset()

		results, err := l.CollapseTransactions(finalized.Index, finalized.Start, finalized.End, true)
		if err != nil {
			fmt.Println(err)
			continue
		}

		if uint64(results.appliedCount) != finalized.Applied {
			fmt.Printf("Expected to have applied %d transactions finalizing a round, but only applied %d transactions instead.\n", finalized.Applied, results.appliedCount)
			continue
		}

		if results.snapshot.Checksum() != finalized.Merkle {
			fmt.Printf("Expected finalized rounds merkle root to be %x, but got %x.\n", finalized.Merkle, results.snapshot.Checksum())
			continue
		}

		pruned, err := l.rounds.Save(finalized)
		if err != nil {
			fmt.Printf("Failed to save finalized round to our database: %v\n", err)
		}

		if pruned != nil {
			count := l.graph.PruneBelowDepth(pruned.End.Depth)

			logger := log.Consensus("prune")
			logger.Debug().
				Int("num_tx", count).
				Uint64("current_round_id", finalized.Index).
				Uint64("pruned_round_id", pruned.Index).
				Msg("Pruned away round and transactions.")
		}

		l.graph.UpdateRoot(finalized.End.Depth)

		if err = l.accounts.Commit(results.snapshot); err != nil {
			fmt.Printf("Failed to commit collaped state to our database: %v\n", err)
		}

		l.metrics.acceptedTX.Mark(int64(results.appliedCount))

		logger := log.Consensus("round_end")
		logger.Info().
			Int("num_applied_tx", results.appliedCount).
			Int("num_rejected_tx", results.rejectedCount).
			Int("num_ignored_tx", results.ignoredCount).
			Uint64("old_round", current.Index).
			Uint64("new_round", finalized.Index).
			Uint8("old_difficulty", current.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
			Uint8("new_difficulty", finalized.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
			Hex("new_root", finalized.End.ID[:]).
			Hex("old_root", current.End.ID[:]).
			Hex("new_merkle_root", finalized.Merkle[:]).
			Hex("old_merkle_root", current.Merkle[:]).
			Uint64("round_depth", finalized.End.Depth-finalized.Start.Depth).
			Msg("Finalized consensus round, and initialized a new round.")

		//go ExportGraphDOT(finalized, l.graph)
	}
}

func (l *Ledger) SyncToLatestRound() {
	voteWG := new(sync.WaitGroup)

	go CollectVotes(l.accounts, l.syncer, l.syncVotes, voteWG)

	for {
		for {
			conns, err := SelectPeers(l.client.ClosestPeers(), sys.SnowballK)
			if err != nil {
				select {
				case <-time.After(1 * time.Second):
				}

				continue
			}

			current := l.rounds.Latest()

			var wg sync.WaitGroup
			wg.Add(len(conns))

			for _, conn := range conns {
				client := NewWaveletClient(conn)

				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

					p := &peer.Peer{}

					res, err := client.CheckOutOfSync(ctx, &OutOfSyncRequest{}, grpc.Peer(p))
					if err != nil {
						cancel()
						wg.Done()
						return
					}

					cancel()

					info := noise.InfoFromPeer(p)
					if info == nil {
						wg.Done()
						return
					}

					voter, ok := info.Get(skademlia.KeyID).(*skademlia.ID)
					if !ok {
						wg.Done()
						return
					}

					round, err := UnmarshalRound(bytes.NewReader(res.Round))
					if err != nil {
						wg.Done()
						return
					}

					if round.ID == ZeroRoundID || round.Start.ID == ZeroTransactionID || round.End.ID == ZeroTransactionID {
						wg.Done()
						return
					}

					if round.Index < sys.SyncIfRoundsDifferBy+current.Index {
						wg.Done()
						return
					}

					l.syncVotes <- vote{voter: voter, preferred: &round}

					wg.Done()
				}()
			}

			wg.Wait()

			if l.syncer.Decided() {
				break
			}

			l.syncTimer.Reset((5000 / (1 + time.Duration(l.syncer.Progress()))) * time.Second)

			select {
			case <-l.syncTimer.C:
			}
		}

		// Wait for all consensus-related workers to close.

		close(l.sync)
		l.consensus.Wait()

		// Wait for the vote processor worker to close.

		voteWG.Add(1)
		close(l.syncVotes)
		voteWG.Wait()

		// Reset all Snowball samplers.

		l.finalizer.Reset()
		l.syncer.Reset()

		// TODO(kenta): Sync here.

		// Spawn a new vote processor worker.

		l.syncVotes = make(chan vote, sys.SnowballK)
		go CollectVotes(l.accounts, l.syncer, l.syncVotes, voteWG)

		// Respawn all consensus-related workers.

		l.sync = make(chan struct{})
		l.PerformConsensus()
	}
}

// ApplyTransactionToSnapshot applies a transactions intended changes to a snapshot
// of the ledgers current state.
func (l *Ledger) ApplyTransactionToSnapshot(snapshot *avl.Tree, tx *Transaction) error {
	ctx := NewTransactionContext(snapshot, tx)

	if err := ctx.apply(l.processors); err != nil {
		return errors.Wrap(err, "could not apply transaction to snapshot")
	}

	return nil
}

// CollapseResults is what is returned by calling CollapseTransactions. Refer to CollapseTransactions
// to understand what counts of accepted, rejected, or otherwise ignored transactions truly represent
// after calling CollapseTransactions.
type CollapseResults struct {
	rejectedCount int
	appliedCount  int
	ignoredCount  int
	snapshot      *avl.Tree
}

// CollapseTransactions takes all transactions recorded within a graph depth interval, and applies
// all valid and available ones to a snapshot of all accounts stored in the ledger. It returns
// an updated snapshot with all finalized transactions applied, alongside count summaries of the
// number of applied, rejected, or otherwise ignored transactions.
//
// Transactions that intersect within all paths from the start to end of a depth interval that are
// also applicable to the ledger state are considered as accepted. Transactions that do not
// intersect with any of the paths from the start to end of a depth interval t all are considered
// as ignored transactions. Transactions that fall entirely out of either applied or ignored are
// considered to be rejected.
//
// It is important to note that transactions that are inspected over are specifically transactions
// that are within the depth interval (start, end] where start is the interval starting point depth,
// and end is the interval ending point depth.
func (l *Ledger) CollapseTransactions(round uint64, start Transaction, end Transaction, logging bool) (CollapseResults, error) {
	if results, exists := l.cacheCollapse.load(end.Seed); exists {
		return results.(CollapseResults), nil
	}

	var results CollapseResults

	results.snapshot = l.accounts.Snapshot()
	results.snapshot.SetViewID(round)

	visited := map[TransactionID]struct{}{start.ID: {}}

	queue := AcquireQueue()
	defer ReleaseQueue(queue)

	queue.PushBack(&end)

	order := AcquireQueue()
	defer ReleaseQueue(order)

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; seen {
				continue
			}

			visited[parentID] = struct{}{}

			parent := l.graph.FindTransaction(parentID)

			if parent == nil {
				l.graph.MarkTransactionAsMissing(parentID, popped.Depth)
				return results, errors.Errorf("missing ancestor %x to correctly collapse down ledger state from critical transaction %x", parentID, end.ID)
			}

			if parent.Depth <= start.Depth {
				continue
			}

			queue.PushBack(parent)
		}

		order.PushBack(popped)
	}

	// Apply transactions in reverse order from the end of the round
	// all the way down to the beginning of the round.

	for order.Len() > 0 {
		popped := order.PopBack().(*Transaction)

		if err := l.RewardValidators(results.snapshot, start, popped, logging); err != nil {
			if logging {
				logEventTX("failed", popped, err)
			}

			results.rejectedCount += popped.LogicalUnits()
			continue
		}

		if err := l.ApplyTransactionToSnapshot(results.snapshot, popped); err != nil {
			if logging {
				logEventTX("failed", popped, err)
			}

			results.rejectedCount += popped.LogicalUnits()
			continue
		}

		if logging {
			logEventTX("applied", popped)
		}

		// Update nonce.

		nonce, _ := ReadAccountNonce(results.snapshot, popped.Creator)
		WriteAccountNonce(results.snapshot, popped.Creator, nonce+1)

		results.appliedCount += popped.LogicalUnits()
	}

	startDepth, endDepth := start.Depth+1, end.Depth

	for _, tx := range l.graph.GetTransactionsByDepth(&startDepth, &endDepth) {
		results.ignoredCount += tx.LogicalUnits()
	}

	results.ignoredCount -= results.appliedCount + results.rejectedCount

	l.cacheCollapse.put(end.Seed, results)
	return results, nil
}

func (l *Ledger) RewardValidators(snapshot *avl.Tree, root Transaction, tx *Transaction, logging bool) error {
	var candidates []*Transaction
	var stakes []uint64
	var totalStake uint64

	visited := make(map[TransactionID]struct{})

	queue := AcquireQueue()
	defer ReleaseQueue(queue)

	for _, parentID := range tx.ParentIDs {
		if parent := l.graph.FindTransaction(parentID); parent != nil {
			queue.PushBack(parent)
		}

		visited[parentID] = struct{}{}
	}

	hasher, _ := blake2b.New256(nil)

	var depthCounter uint64
	var lastDepth = tx.Depth

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

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
			stake, _ := ReadAccountStake(snapshot, popped.Sender)

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
				if parent := l.graph.FindTransaction(parentID); parent != nil {
					queue.PushBack(parent)
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

	creatorBalance, _ := ReadAccountBalance(snapshot, tx.Creator)
	recipientBalance, _ := ReadAccountBalance(snapshot, rewardee.Sender)

	fee := sys.TransactionFeeAmount

	if creatorBalance < fee {
		return errors.Errorf("stake: creator %x does not have enough PERLs to pay transaction fees (requested %d PERLs) to %x", tx.Creator, fee, rewardee.Sender)
	}

	WriteAccountBalance(snapshot, tx.Creator, creatorBalance-fee)
	WriteAccountBalance(snapshot, rewardee.Sender, recipientBalance+fee)

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
