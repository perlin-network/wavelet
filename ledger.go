// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package wavelet

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/lru"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/willf/bloom"
	"golang.org/x/crypto/blake2b"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/peer"
)

type bitset uint8

const (
	outOfSync   bitset = 0
	synced             = 1
	finalized          = 2
	fullySynced        = 3
)

var (
	ErrOutOfSync = errors.New("Node is currently ouf of sync. Please try again later.")

	EmptyBlockID [blake2b.Size256]byte
)

type Ledger struct {
	client  *skademlia.Client
	metrics *Metrics
	indexer *Indexer

	accounts *Accounts
	blocks   *Blocks

	mempool *Mempool

	finalizer *Snowball
	syncer    *Snowball

	consensus sync.WaitGroup

	broadcastNops         bool
	broadcastNopsMaxDepth uint64
	broadcastNopsDelay    time.Time
	broadcastNopsLock     sync.Mutex

	sync chan struct{}

	syncStatus     bitset
	syncStatusLock sync.RWMutex

	cacheCollapse *lru.LRU
	fileBuffers   *fileBufferPool

	sendQuota chan struct{}

	stallDetector *StallDetector

	stop     chan struct{}
	stopWG   sync.WaitGroup
	cancelGC context.CancelFunc

	transactionIDs *bloom.BloomFilter
}

type config struct {
	GCDisabled  bool
	Genesis     *string
	MaxMemoryMB uint64
}

type Option func(cfg *config)

// WithoutGC disables GC. Used for testing purposes.
func WithoutGC() Option {
	return func(cfg *config) {
		cfg.GCDisabled = true
	}
}

func WithGenesis(genesis *string) Option {
	return func(cfg *config) {
		cfg.Genesis = genesis
	}
}

func WithMaxMemoryMB(n uint64) Option {
	return func(cfg *config) {
		cfg.MaxMemoryMB = n
	}
}

func NewLedger(kv store.KV, client *skademlia.Client, opts ...Option) *Ledger {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	logger := log.Node()

	metrics := NewMetrics(context.TODO())
	indexer := NewIndexer()

	accounts := NewAccounts(kv)
	blocks, err := NewBlocks(kv, conf.GetPruningLimit())

	var block *Block

	if blocks != nil && err != nil {
		genesis := performInception(accounts.tree, cfg.Genesis)
		if err := accounts.Commit(nil); err != nil {
			logger.Fatal().Err(err).Msg("BUG: accounts.Commit")
		}

		ptr := &genesis

		if _, err := blocks.Save(ptr); err != nil {
			logger.Fatal().Err(err).Msg("BUG: blocks..Save")
		}

		block = ptr
	} else if blocks != nil {
		block = blocks.Latest()
	}

	if block == nil {
		logger.Fatal().Err(err).Msg("BUG: COULD NOT FIND GENESIS, OR STORAGE IS CORRUPTED.")
	}

	transactionIDs := bloom.New(conf.GetBloomFilterM(), conf.GetBloomFilterK())

	finalizer := NewSnowball(WithName("finalizer"))
	syncer := NewSnowball(WithName("syncer"))

	ledger := &Ledger{
		client:  client,
		metrics: metrics,
		indexer: indexer,

		accounts: accounts,
		blocks:   blocks,

		mempool: NewMempool(),

		finalizer: finalizer,
		syncer:    syncer,

		syncStatus: finalized, // we start node as out of sync, but finalized

		sync: make(chan struct{}),

		cacheCollapse: lru.NewLRU(16),
		fileBuffers:   newFileBufferPool(sys.SyncPooledFileSize, ""),

		sendQuota: make(chan struct{}, 2000),

		transactionIDs: transactionIDs,
	}

	if !cfg.GCDisabled {
		ctx, cancel := context.WithCancel(context.Background())
		go accounts.GC(ctx, &ledger.stopWG)

		ledger.cancelGC = cancel
	}

	stop := make(chan struct{}) // TODO: Real graceful stop.
	var stallDetector *StallDetector
	stallDetector = NewStallDetector(stop, StallDetectorConfig{
		MaxMemoryMB: cfg.MaxMemoryMB,
	}, StallDetectorDelegate{
		PrepareShutdown: func(err error) {
			logger := log.Node()
			logger.Error().Err(err).Msg("Shutting down node...")
		},
	})
	go stallDetector.Run(&ledger.stopWG)

	ledger.stallDetector = stallDetector
	ledger.stop = stop

	ledger.PerformConsensus()

	go ledger.SyncToLatestRound()
	go ledger.PushSendQuota()

	return ledger
}

// Close stops all goroutines and waits for them to complete.
func (l *Ledger) Close() {
	syncOpen := true
	select {
	case _, syncOpen = <-l.sync:
	default:
	}

	if syncOpen {
		close(l.sync)
	}

	l.consensus.Wait()

	close(l.stop)

	if l.cancelGC != nil {
		l.cancelGC()
	}

	l.stopWG.Wait()
}

// AddTransaction adds a transaction to the ledger. If the transaction is
// invalid or fails any validation checks, an error is returned. No error
// is returned if the transaction has already existed int he ledgers graph
// beforehand.
func (l *Ledger) AddTransaction(tx Transaction) error {

	err := l.mempool.Add(tx, l.LastBlockID())

	// Ignore error if transaction already exists,
	// or transaction's depth is too low due to pruning
	if err != nil &&
		errors.Cause(err) != ErrAlreadyExists &&
		errors.Cause(err) != ErrDepthTooLow {

		return err
	}

	if err == nil {
		l.transactionIDs.Add(tx.ID[:])

		l.TakeSendQuota()

		l.broadcastNopsLock.Lock()
		if tx.Tag != sys.TagNop && tx.Sender == l.client.Keys().PublicKey() {
			l.broadcastNops = true
			l.broadcastNopsDelay = time.Now()

			if tx.Depth > l.broadcastNopsMaxDepth {
				l.broadcastNopsMaxDepth = tx.Depth
			}
		}
		l.broadcastNopsLock.Unlock()
	}

	return nil
}

// Find searches through complete transaction and account indices for a specified
// query string. All indices that queried are in the form of tries. It is safe
// to call this method concurrently.
func (l *Ledger) Find(query string, max int) (results []string) {
	var err error

	if max > 0 {
		results = make([]string, 0, max)
	}

	prefix := []byte(query)

	if len(query)%2 == 1 { // Cut off a single character.
		prefix = prefix[:len(prefix)-1]
	}

	prefix, err = hex.DecodeString(string(prefix))
	if err != nil {
		return nil
	}

	bucketPrefix := append(keyAccounts[:], keyAccountNonce[:]...)
	fullQuery := append(bucketPrefix, prefix...)

	l.Snapshot().IterateFrom(fullQuery, func(key, _ []byte) bool {
		if !bytes.HasPrefix(key, fullQuery) {
			return false
		}

		if max > 0 && len(results) >= max {
			return false
		}

		results = append(results, hex.EncodeToString(key[len(bucketPrefix):]))
		return true
	})

	var count = -1
	if max > 0 {
		count = max - len(results)
	}

	return append(results, l.indexer.Find(query, count)...)
}

// PushSendQuota permits one token into this nodes send quota bucket every millisecond
// such that the node may add one single transaction into its graph.
func (l *Ledger) PushSendQuota() {
	for range time.Tick(1 * time.Millisecond) {
		select {
		case l.sendQuota <- struct{}{}:
		default:
		}
	}
}

// TakeSendQuota removes one token from this nodes send quota bucket to signal
// that the node has added one single transaction into its graph.
func (l *Ledger) TakeSendQuota() bool {
	select {
	case <-l.sendQuota:
		return true
	default:
		return false
	}
}

// Protocol returns an implementation of WaveletServer to handle incoming
// RPC and streams for the ledger. The protocol is agnostic to whatever
// choice of network stack is used with Wavelet, though by default it is
// intended to be used with gRPC and Noise.
func (l *Ledger) Protocol() *Protocol {
	return &Protocol{ledger: l}
}

// Finalizer returns the Snowball finalizer which finalizes the contents of individual
// consensus rounds.
func (l *Ledger) Finalizer() *Snowball {
	return l.finalizer
}

// Rounds returns the round manager for the ledger.
func (l *Ledger) Blocks() *Blocks {
	return l.blocks
}

// Restart restart wavelet process by means of stall detector (approach is platform dependent)
func (l *Ledger) Restart() error {
	return l.stallDetector.tryRestart()
}

// PerformConsensus spawns workers related to performing consensus, such as pulling
// missing transactions and incrementally finalizing intervals of transactions in
// the ledgers graph.
func (l *Ledger) PerformConsensus() {
	go l.PullTransactions()
	go l.FinalizeRounds()
}

func (l *Ledger) Snapshot() *avl.Tree {
	return l.accounts.Snapshot()
}

// BroadcastingNop returns true if the node is
// supposed to broadcast nop transaction.
func (l *Ledger) BroadcastingNop() bool {
	l.broadcastNopsLock.Lock()
	broadcastNops := l.broadcastNops
	l.broadcastNopsLock.Unlock()

	return broadcastNops
}

// BroadcastNop has the node send a nop transaction should they have sufficient
// balance available. They are broadcasted if no other transaction that is not a nop transaction
// is not broadcasted by the node after 500 milliseconds. These conditions only apply so long as
// at least one transaction gets broadcasted by the node within the current round. Once a round
// is tentatively being finalized, a node will stop broadcasting nops.
func (l *Ledger) BroadcastNop() *Transaction {
	l.broadcastNopsLock.Lock()
	broadcastNops := l.broadcastNops
	broadcastNopsDelay := l.broadcastNopsDelay
	l.broadcastNopsLock.Unlock()

	if !broadcastNops || time.Now().Sub(broadcastNopsDelay) < 100*time.Millisecond {
		return nil
	}

	keys := l.client.Keys()
	publicKey := keys.PublicKey()

	balance, _ := ReadAccountBalance(l.accounts.Snapshot(), publicKey)

	// FIXME(kenta): FOR TESTNET ONLY. FAUCET DOES NOT GET ANY PERLs DEDUCTED.
	if balance < sys.DefaultTransactionFee && hex.EncodeToString(publicKey[:]) != sys.FaucetAddress {
		return nil
	}

	nop := AttachSenderToTransaction(keys, NewTransaction(sys.TagNop, nil), l.graph.FindEligibleParents()...)

	if err := l.AddTransaction(nop); err != nil {
		return nil
	}

	return l.graph.FindTransaction(nop.ID)
}

// PullTransactions is a goroutine which continuously pulls missing transactions
// from randomly sampled peers in the network. It does so by sending a list of
// transaction IDs it has, and the peer will respond with a list of transactions
// which the peer has but the sender node doesn't.
//
// The transaction IDs are stored inside a bloom filter to keep the space constant
// with increasing transactions, and also because bloom filter has no false negative
// (the peer will only use it to check if a transaction is not in the set).
func (l *Ledger) PullTransactions() {
	l.consensus.Add(1)
	defer l.consensus.Done()

	for {
		select {
		case <-l.sync:
			return

		default:
		}

		closestPeers := l.client.ClosestPeers()

		peers := make([]*grpc.ClientConn, 0, len(closestPeers))
		for _, p := range closestPeers {
			if p.GetState() == connectivity.Ready {
				peers = append(peers, p)
			}
		}

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

		logger := log.Consensus("pull-transactions")

		// Marshal the bloom filter
		buf := new(bytes.Buffer)
		if _, err := l.transactionIDs.WriteTo(buf); err != nil {
			logger.Error().Err(err).Msg("failed to marshal bloom filter")
			continue
		}

		req := &DownloadMissingTxRequest{TransactionIds: buf.Bytes()}

		conn := peers[0]
		client := NewWaveletClient(conn)

		ctx, cancel := context.WithTimeout(context.Background(), conf.GetDownloadTxTimeout())
		batch, err := client.DownloadMissingTx(ctx, req)
		if err != nil {
			logger.Error().Err(err).Msg("failed to download missing transactions")
			cancel()
			continue
		}
		cancel()

		count := int64(0)

		for _, buf := range batch.Transactions {
			tx, err := UnmarshalTransaction(bytes.NewReader(buf))
			if err != nil {
				logger.Error().
					Err(err).
					Hex("tx_id", tx.ID[:]).
					Msg("error unmarshaling downloaded tx")
				continue
			}

			if err := l.AddTransaction(tx); err != nil && errors.Cause(err) != ErrMissingParents {
				logger.Error().
					Err(err).
					Hex("tx_id", tx.ID[:]).
					Msg("error adding downloaded tx to graph")
				continue
			}

			count += int64(tx.LogicalUnits())
		}

		l.metrics.downloadedTX.Mark(count)
		l.metrics.receivedTX.Mark(count)
	}
}

func (l *Ledger) FinalizeBlocks() {
	for {
		preferred := l.finalizer.Preferred()
		decided := l.finalizer.Decided()

		if preferred == nil {
			// TODO propose a block
		} else {
			if decided {
				l.finalize(*preferred.(*Block))
			} else {
				l.query()
			}
		}

	}
}

func (l *Ledger) finalize(newBlock Block) {
	// TODO delete transactions, shuffle mempool, collapse transactions, reset snowbal
}

func (l *Ledger) query() {
	// TODO check there's enough peers

	current := l.blocks.Latest()

	workerChan := make(chan *grpc.ClientConn, 16)

	var workerWG sync.WaitGroup
	workerWG.Add(cap(workerChan))

	snowballK := conf.GetSnowballK()
	voteChan := make(chan finalizationVote, snowballK)

	req := &QueryRequest{BlockIndex: current.Index + 1}

	for i := 0; i < cap(workerChan); i++ {
		go func() {
			for conn := range workerChan {
				f := func() {
					client := NewWaveletClient(conn)

					ctx, cancel := context.WithTimeout(context.Background(), conf.GetQueryTimeout())

					p := &peer.Peer{}

					res, err := client.Query(ctx, req, grpc.Peer(p))
					if err != nil {
						logger := log.Node()
						logger.Error().
							Err(err).
							Msg("error while querying peer")

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

					block, err := UnmarshalBlock(bytes.NewReader(res.Block))
					if err != nil {
						voteChan <- finalizationVote{voter: voter, block: nil}
						return
					}

					if block.ID == ZeroRoundID {
						voteChan <- finalizationVote{voter: voter, block: nil}
						return
					}

					voteChan <- finalizationVote{
						voter: voter,
						block: &block,
					}
				}
				l.metrics.queryLatency.Time(f)
			}
			workerWG.Done()
		}()
	}

	cleanupWorker := func() {
		close(workerChan)
		workerWG.Wait()
	}

	peers, err := SelectPeers(l.client.ClosestPeers(), conf.GetSnowballK())
	if err != nil {
		cleanupWorker()
		return
	}

	for _, p := range peers {
		workerChan <- p
	}

	votes := make([]finalizationVote, 0, snowballK)
	voters := make(map[AccountID]struct{}, snowballK)

	for response := range voteChan {
		if _, recorded := voters[response.voter.PublicKey()]; recorded {
			continue // To make sure the sampling process is fair, only allow one vote per peer.
		}

		voters[response.voter.PublicKey()] = struct{}{}
		votes = append(votes, response)

		if len(votes) == cap(votes) {
			break
		}
	}

	cleanupWorker()

	if len(votes) != cap(votes) {
		// TODO not enough vote, what to do ?
		return
	}

	// Filter away all query responses whose blocks comprise of transactions our node is not aware of.
	l.mempool.ReadLock(func(transactions map[[32]byte]*Transaction) {
		for _, vote := range votes {
			if vote.block == nil {
				continue
			}

			if vote.block.ID == ZeroBlockID {
				continue
			}

			if vote.block.Index != l.LastBlockIndex() {
				vote.block.ID = ZeroBlockID
				continue
			}

			for _, id := range vote.block.Transactions {
				if _, stored := transactions[id]; !stored {
					vote.block.ID = ZeroBlockID
					break
				}
			}
		}
	})

	tallies := make(map[[blake2b.Size256]byte]float64)
	blocks := make(map[[blake2b.Size256]byte]*Block)

	for _, vote := range votes {
		if vote.block == nil {
			continue
		}

		if _, exists := blocks[vote.block.ID]; !exists {
			blocks[vote.block.ID] = vote.block
		}

		tallies[vote.block.ID] += 1.0 / float64(len(votes))
	}

	for block, weight := range Normalize(ComputeProfitWeights(votes)) {
		tallies[block] *= weight
	}

	stakeWeights := Normalize(ComputeStakeWeights(l.accounts, votes))
	for block, weight := range stakeWeights {
		tallies[block] *= weight
	}

	totalTally := float64(0)
	for _, tally := range tallies {
		totalTally += tally
	}

	for block := range tallies {
		tallies[block] /= totalTally
	}

	snowballTallies := make(map[Identifiable]float64, len(blocks))
	for _, block := range blocks {
		snowballTallies[block] = tallies[block.ID]
	}

	l.finalizer.Tick(snowballTallies)
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

		if len(l.client.ClosestPeerIDs()) < conf.GetSnowballK() {
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
				nop := l.BroadcastNop()

				if nop != nil {
					if !nop.IsCritical(currentDifficulty) {
						select {
						case <-l.sync:
							return
						case <-time.After(500 * time.Microsecond):
						}
					}

					continue FINALIZE_ROUNDS
				}

				select {
				case <-l.sync:
					return
				case <-time.After(100 * time.Millisecond):
				}

				continue FINALIZE_ROUNDS
			}

			results, err := l.collapseTransactions(current.Index+1, current.End, *eligible, false)
			if err != nil {
				logger := log.Node()
				logger.Error().
					Err(err).
					Msg("error collapsing transactions during finalization")
				continue
			}

			candidate := NewRound(current.Index+1, results.snapshot.Checksum(), uint32(results.appliedCount+results.rejectedCount), current.End, *eligible)
			l.finalizer.Prefer(&candidate)

			continue FINALIZE_ROUNDS
		}

		// Only stop broadcasting nops if the most recently added transaction
		// has been applied
		l.broadcastNopsLock.Lock()
		preferred := l.finalizer.Preferred().(*Round)
		if l.broadcastNops && l.broadcastNopsMaxDepth <= preferred.End.Depth {
			l.broadcastNops = false
		}
		l.broadcastNopsLock.Unlock()

		workerChan := make(chan *grpc.ClientConn, 16)

		var workerWG sync.WaitGroup
		workerWG.Add(cap(workerChan))

		snowballK := conf.GetSnowballK()
		voteChan := make(chan finalizationVote, snowballK)
		go CollectVotesForFinalization(l.accounts, l.finalizer, voteChan, &workerWG, snowballK)

		req := &QueryRequest{RoundIndex: current.Index + 1}

		for i := 0; i < cap(workerChan); i++ {
			go func() {
				for conn := range workerChan {
					f := func() {
						client := NewWaveletClient(conn)

						ctx, cancel := context.WithTimeout(context.Background(), conf.GetQueryTimeout())

						p := &peer.Peer{}

						res, err := client.Query(ctx, req, grpc.Peer(p))
						if err != nil {
							logger := log.Node()
							logger.Error().
								Err(err).
								Msg("error while querying peer")

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
							voteChan <- finalizationVote{voter: voter, round: ZeroRoundPtr}
							return
						}

						if round.ID == ZeroRoundID || round.Start.ID == ZeroTransactionID || round.End.ID == ZeroTransactionID {
							voteChan <- finalizationVote{voter: voter, round: ZeroRoundPtr}
							return
						}

						if round.End.Depth <= round.Start.Depth {
							return
						}

						if round.Index != current.Index+1 {
							return
						}

						if round.Start.ID != current.End.ID {
							return
						}

						if err := l.AddTransaction(round.Start); err != nil {
							return
						}

						if err := l.AddTransaction(round.End); err != nil {
							return
						}

						if !round.End.IsCritical(currentDifficulty) {
							return
						}

						results, err := l.collapseTransactions(round.Index, round.Start, round.End, false)
						if err != nil {
							if !strings.Contains(err.Error(), "missing ancestor") {
								logger := log.Node()
								logger.Error().
									Err(err).
									Msg("error collapsing transactions")
							}
							return
						}

						if uint32(results.appliedCount+results.rejectedCount) != round.Transactions {
							logger := log.Node()
							logger.Error().
								Err(err).
								Uint32("expected", round.Transactions).
								Int("actual", results.appliedCount).
								Int("rejected", results.rejectedCount).
								Int("ignored", results.ignoredCount).
								Msg("Number of applied transactions does not match")
							return
						}

						if results.snapshot.Checksum() != round.Merkle {
							actualMerkle := results.snapshot.Checksum()
							logger := log.Node()
							logger.Error().
								Err(err).
								Hex("expected", round.Merkle[:]).
								Hex("actual", actualMerkle[:]).
								Msg("Unexpected round merkle root")
							return
						}

						voteChan <- finalizationVote{
							voter: voter,
							round: &round,
						}
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
				workerWG.Wait() // Wait for vote processor worker to stop

				return
			default:
			}

			// Randomly sample a peer to query
			peers, err := SelectPeers(l.client.ClosestPeers(), conf.GetSnowballK())
			if err != nil {
				close(workerChan)
				workerWG.Wait()
				workerWG.Add(1)
				close(voteChan)
				workerWG.Wait() // Wait for vote processor worker to stop

				continue FINALIZE_ROUNDS
			}

			for _, p := range peers {
				workerChan <- p
			}
		}

		close(workerChan)
		workerWG.Wait() // Wait for query workers to stop
		workerWG.Add(1)
		close(voteChan)
		workerWG.Wait() // Wait for vote processor worker to stop

		preferred = l.finalizer.Preferred().(*Round)
		l.finalizer.Reset()

		results, err := l.collapseTransactions(preferred.Index, preferred.Start, preferred.End, true)
		if err != nil {
			if !strings.Contains(err.Error(), "missing ancestor") {
				logger := log.Node()
				logger.Error().
					Err(err).
					Msg("error collapsing transactions during finalization")
			}
			continue
		}

		if uint32(results.appliedCount+results.rejectedCount) != preferred.Transactions {
			logger := log.Node()
			logger.Error().
				Err(err).
				Uint32("expected", preferred.Transactions).
				Int("actual", results.appliedCount).
				Msg("Number of applied transactions does not match")
			continue
		}

		if results.snapshot.Checksum() != preferred.Merkle {
			logger := log.Node()
			actualMerkle := results.snapshot.Checksum()
			logger.Error().
				Err(err).
				Hex("expected", preferred.Merkle[:]).
				Hex("actual", actualMerkle[:]).
				Msg("Unexpected round merkle root")
			continue
		}

		pruned, err := l.rounds.Save(preferred)
		if err != nil {
			logger := log.Node()
			logger.Error().
				Err(err).
				Msg("Failed to save preferred round to our database")
		}

		if pruned != nil {
			count := l.graph.PruneBelowDepth(pruned.End.Depth)
			l.rebuildBloomFilter()

			logger := log.Consensus("prune")
			logger.Debug().
				Int("num_tx", count).
				Uint64("current_round_id", preferred.Index).
				Uint64("pruned_round_id", pruned.Index).
				Msg("Pruned away round and transactions.")
		}

		l.graph.UpdateRootDepth(preferred.End.Depth)

		if err = l.accounts.Commit(results.snapshot); err != nil {
			logger := log.Node()
			logger.Error().
				Err(err).
				Msg("Failed to commit collaped state to our database")
		}

		l.metrics.acceptedTX.Mark(int64(results.appliedCount))

		l.LogChanges(results.snapshot, current.Index)

		l.applySync(finalized)

		logger := log.Consensus("round_end")
		logger.Info().
			Int("num_applied_tx", results.appliedCount).
			Int("num_rejected_tx", results.rejectedCount).
			Int("num_ignored_tx", results.ignoredCount).
			Uint64("old_round", current.Index).
			Uint64("new_round", preferred.Index).
			Uint8("old_difficulty", current.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
			Uint8("new_difficulty", preferred.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
			Hex("new_root", preferred.End.ID[:]).
			Hex("old_root", current.End.ID[:]).
			Hex("new_merkle_root", preferred.Merkle[:]).
			Hex("old_merkle_root", current.Merkle[:]).
			Uint64("round_depth", preferred.End.Depth-preferred.Start.Depth).
			Msg("Finalized consensus round, and initialized a new round.")
	}
}

// rebuildBloomFilter rebuilds the bloom filter which stores the set of
// transactions in the graph. This method is called after transactions are
// pruned, as it's not possible to remove items from a bloom filter.
func (l *Ledger) rebuildBloomFilter() {
	l.transactionIDs.ClearAll()

	transactions := l.graph.GetTransactionsByDepth(nil, nil)
	for _, tx := range transactions {
		l.transactionIDs.Add(tx.ID[:])
	}
}

type outOfSyncVote struct {
	outOfSync bool
}

func (o *outOfSyncVote) GetID() string {
	return fmt.Sprintf("%v", o.outOfSync)
}

func (l *Ledger) SyncToLatestRound() {
	voteWG := new(sync.WaitGroup)

	snowballK := conf.GetSnowballK()
	syncVotes := make(chan syncVote, snowballK)

	logger := log.Sync("sync")

	go CollectVotesForSync(l.accounts, l.syncer, syncVotes, voteWG, snowballK)

	syncTimeoutMultiplier := 0
	for {
		for {
			time.Sleep(5 * time.Millisecond)

			conns, err := SelectPeers(l.client.ClosestPeers(), conf.GetSnowballK())
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
				go func(conn *grpc.ClientConn) {
					client := NewWaveletClient(conn)

					ctx, cancel := context.WithTimeout(context.Background(), conf.GetCheckOutOfSyncTimeout())

					p := &peer.Peer{}

					res, err := client.CheckOutOfSync(
						ctx,
						&OutOfSyncRequest{RoundIndex: current.Index},
						grpc.Peer(p),
					)
					if err != nil {
						logger.Error().
							Err(err).
							Msgf("error while checking out of sync with %v", p.Addr)

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

					syncVotes <- syncVote{voter: voter, outOfSync: res.OutOfSync}

					wg.Done()
				}(conn)
			}

			wg.Wait()

			if l.syncer.Decided() {
				break
			}
		}

		// Reset syncing Snowball sampler. Check if it is a false alarm such that we don't have to sync.

		current := l.rounds.Latest()
		preferred := l.syncer.Preferred()

		oos := preferred.(*outOfSyncVote).outOfSync
		if !oos {
			l.applySync(synced)
			l.syncer.Reset()

			if syncTimeoutMultiplier < 60 {
				syncTimeoutMultiplier++
			}

			//logger.Debug().Msgf("Not out of sync, sleeping %d seconds", syncTimeoutMultiplier)

			time.Sleep(time.Duration(syncTimeoutMultiplier) * time.Second)

			continue
		}

		l.setSync(outOfSync)

		syncTimeoutMultiplier = 0

		shutdown := func() {
			close(l.sync)
			l.consensus.Wait() // Wait for all consensus-related workers to shutdown.

			voteWG.Add(1)
			close(syncVotes)
			voteWG.Wait() // Wait for the vote processor worker to shutdown.

			l.finalizer.Reset() // Reset consensus Snowball sampler.
			l.syncer.Reset()    // Reset syncing Snowball sampler.
		}

		restart := func() { // Respawn all previously stopped workers.
			snowballK := conf.GetSnowballK()

			syncVotes = make(chan syncVote, snowballK)
			go CollectVotesForSync(l.accounts, l.syncer, syncVotes, voteWG, snowballK)

			l.sync = make(chan struct{})
			l.PerformConsensus()
		}

		shutdown() // Shutdown all consensus-related workers.

		logger.Info().
			Uint64("current_round", current.Index).
			Msg("Noticed that we are out of sync; downloading latest state Snapshot from our peer(s).")

	SYNC:
		conns, err := SelectPeers(l.client.ClosestPeers(), conf.GetSnowballK())
		if err != nil {
			logger.Warn().Msg("It looks like there are no peers for us to sync with. Retrying...")

			time.Sleep(1 * time.Second)

			goto SYNC
		}

		req := &SyncRequest{Data: &SyncRequest_RoundId{RoundId: current.Index}}

		type response struct {
			header *SyncInfo
			latest Round
			stream Wavelet_SyncClient
		}

		responses := make([]response, 0, len(conns))

		for _, conn := range conns {
			stream, err := NewWaveletClient(conn).Sync(context.Background())
			if err != nil {
				continue
			}

			if err := stream.Send(req); err != nil {
				continue
			}

			res, err := stream.Recv()
			if err != nil {
				continue
			}

			header := res.GetHeader()

			if header == nil {
				continue
			}

			latest, err := UnmarshalRound(bytes.NewReader(header.LatestRound))
			if err != nil {
				continue
			}

			if latest.Index == 0 || len(header.Checksums) == 0 {
				continue
			}

			responses = append(responses, response{header: header, latest: latest, stream: stream})
		}

		if len(responses) == 0 {
			goto SYNC
		}

		dispose := func() {
			for _, res := range responses {
				if err := res.stream.CloseSend(); err != nil {
					continue
				}
			}
		}

		set := make(map[uint64][]response)

		for _, v := range responses {
			set[v.latest.Index] = append(set[v.latest.Index], v)
		}

		// Select a round to sync to which the majority of peers are on.

		var latest *Round
		var majority []response

		for _, votes := range set {
			if len(votes) >= len(set)*2/3 {
				latest = &votes[0].latest
				majority = votes
				break
			}
		}

		// If there is no majority or a tie, dispose all streams and try again.

		if majority == nil {
			logger.Warn().Msg("It looks like our peers could not decide on what the latest round currently is. Retrying...")

			dispose()
			goto SYNC
		}

		logger.Debug().
			Uint64("latest_round", latest.Index).
			Hex("latest_round_root", latest.End.ID[:]).
			Msg("Discovered the round which the majority of our peers are currently in.")

		type source struct {
			idx      int
			checksum [blake2b.Size256]byte
			streams  []Wavelet_SyncClient
			size     int
		}

		var sources []source

		idx := 0

		// For each chunk checksum from the set of checksums provided by each
		// peer, pick the majority checksum.

		for {
			set := make(map[[blake2b.Size256]byte][]Wavelet_SyncClient)

			for _, response := range majority {
				if idx >= len(response.header.Checksums) {
					continue
				}

				var checksum [blake2b.Size256]byte
				copy(checksum[:], response.header.Checksums[idx])

				set[checksum] = append(set[checksum], response.stream)
			}

			if len(set) == 0 {
				break // We have finished going through all responses. Engage in syncing.
			}

			// Figure out what the majority of peers believe the checksum is for a chunk
			// at index idx. If a majority is found, mark the peers as a viable source
			// for grabbing the chunks contents to then reassemble together an AVL+ tree
			// diff to apply to our ledger state to complete syncing.

			consistent := false

			for checksum, voters := range set {
				if len(voters) == 0 || len(voters) < len(majority)*2/3 {
					continue
				}

				sources = append(sources, source{idx: idx, checksum: checksum, streams: voters})
				consistent = true
				break
			}

			// If peers could not come up with a consistent checksum for some
			// chunk at a consistent idx, dispose all streams and try again.

			if !consistent {
				dispose()
				goto SYNC
			}

			idx++
		}

		// Streams may not concurrently send and receive messages at once.
		streamLocks := make(map[Wavelet_SyncClient]*sync.Mutex)
		var streamLock sync.Mutex

		workers := make(chan source, 16)

		var workerWG sync.WaitGroup
		workerWG.Add(cap(workers))

		var chunkWG sync.WaitGroup
		chunkWG.Add(len(sources))

		logger.Debug().
			Int("num_chunks", len(sources)).
			Int("num_workers", cap(workers)).
			Msg("Starting up workers to downloaded all chunks of data needed to sync to the latest round...")

		chunksBuffer, err := l.fileBuffers.GetBounded(int64(len(sources)) * sys.SyncChunkSize)
		if err != nil {
			logger.Error().
				Err(err).
				Msg("Could not create paged buffer! Retrying...")
			goto SYNC
		}

		diffBuffer := l.fileBuffers.GetUnbounded()

		cleanup := func() {
			l.fileBuffers.Put(chunksBuffer)
			l.fileBuffers.Put(diffBuffer)
		}

		for i := 0; i < cap(workers); i++ {
			go func() {
				for src := range workers {
					req := &SyncRequest{Data: &SyncRequest_Checksum{Checksum: src.checksum[:]}}

					for i := 0; i < len(src.streams); i++ {
						stream := src.streams[rand.Intn(len(src.streams))]

						// Lock the stream so that other workers may not concurrently interact
						// with the exact same stream at once.

						streamLock.Lock()
						if _, exists := streamLocks[stream]; !exists {
							streamLocks[stream] = new(sync.Mutex)
						}
						lock := streamLocks[stream]
						streamLock.Unlock()

						lock.Lock()

						if err := stream.Send(req); err != nil {
							lock.Unlock()
							continue
						}

						res, err := stream.Recv()
						if err != nil {
							lock.Unlock()
							continue
						}

						lock.Unlock()

						chunk := res.GetChunk()
						if chunk == nil {
							continue
						}

						if len(chunk) > conf.GetSyncChunkSize() {
							continue
						}

						if blake2b.Sum256(chunk[:]) != src.checksum {
							continue
						}

						// We found the chunk! Store the chunks contents.
						if _, err := chunksBuffer.WriteAt(chunk, int64(src.idx)*sys.SyncChunkSize); err != nil {
							continue
						}

						sources[src.idx].size = len(chunk)

						break
					}

					chunkWG.Done()
				}

				workerWG.Done()
			}()
		}

		for _, src := range sources {
			workers <- src
		}

		chunkWG.Wait() // Wait until all chunks have been received.
		close(workers)
		workerWG.Wait() // Wait until all workers have been closed.

		logger.Debug().
			Int("num_chunks", len(sources)).
			Int("num_workers", cap(workers)).
			Msg("Downloaded whatever chunks were available to sync to the latest round, and shutted down all workers. Checking validity of chunks...")

		dispose() // Shutdown all streams as we no longer need them.

		// Check all chunks has been received
		var diffSize int64
		for i, src := range sources {
			if src.size == 0 {
				logger.Error().
					Uint64("target_round", latest.Index).
					Hex("chunk_checksum", sources[i].checksum[:]).
					Msg("Could not download one of the chunks necessary to sync to the latest round! Retrying...")

				cleanup()
				goto SYNC
			}

			diffSize += int64(src.size)
		}

		if _, err := io.CopyN(diffBuffer, chunksBuffer, diffSize); err != nil {
			logger.Error().
				Uint64("target_round", latest.Index).
				Err(err).
				Msg("Failed to write chunks to bounded memory buffer. Restarting sync...")

			cleanup()
			goto SYNC
		}

		logger.Info().
			Int("num_chunks", len(sources)).
			Uint64("target_round", latest.Index).
			Msg("All chunks have been successfully verified and re-assembled into a diff. Applying diff...")

		snapshot := l.accounts.Snapshot()
		if err := snapshot.ApplyDiff(diffBuffer); err != nil {
			logger.Error().
				Uint64("target_round", latest.Index).
				Err(err).
				Msg("Failed to apply re-assembled diff to our ledger state. Restarting sync...")

			cleanup()
			goto SYNC
		}

		if checksum := snapshot.Checksum(); checksum != latest.Merkle {
			logger.Error().
				Uint64("target_round", latest.Index).
				Hex("expected_merkle_root", latest.Merkle[:]).
				Hex("yielded_merkle_root", checksum[:]).
				Msg("Failed to apply re-assembled diff to our ledger state. Restarting sync...")

			cleanup()
			goto SYNC
		}

		pruned, err := l.rounds.Save(latest)
		if err != nil {
			logger.Error().
				Err(err).
				Msg("Failed to save finalized round to our database")

			cleanup()
			goto SYNC
		}

		if pruned != nil {
			count := l.graph.PruneBelowDepth(pruned.End.Depth)
			l.rebuildBloomFilter()

			logger := log.Consensus("prune")
			logger.Debug().
				Int("num_tx", count).
				Uint64("current_round_id", latest.Index).
				Uint64("pruned_round_id", pruned.Index).
				Msg("Pruned away round and transactions.")
		}

		l.graph.UpdateRoot(latest.End)

		if err := l.accounts.Commit(snapshot); err != nil {
			cleanup()
			logger := log.Node()
			logger.Fatal().Err(err).Msg("failed to commit collapsed state to our database")
		}

		logger = log.Sync("apply")
		logger.Info().
			Int("num_chunks", len(sources)).
			Uint64("old_round", current.Index).
			Uint64("new_round", latest.Index).
			Uint8("old_difficulty", current.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
			Uint8("new_difficulty", latest.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
			Hex("new_root", latest.End.ID[:]).
			Hex("old_root", current.End.ID[:]).
			Hex("new_merkle_root", latest.Merkle[:]).
			Hex("old_merkle_root", current.Merkle[:]).
			Msg("Successfully built a new state Snapshot out of chunk(s) we have received from peers.")

		cleanup()
		restart()
	}
}

// collapseResults is what returned by calling collapseTransactions. Refer to collapseTransactions
// to understand what counts of accepted, rejected, or otherwise ignored transactions truly represent
// after calling collapseTransactions.
type collapseResults struct {
	applied        []*Transaction
	rejected       []*Transaction
	rejectedErrors []error

	appliedCount  int
	rejectedCount int
	ignoredCount  int

	snapshot *avl.Tree
}

type CollapseState struct {
	once    sync.Once
	results *collapseResults
	err     error
}

// collapseTransactions takes all transactions recorded within a graph depth interval, and applies
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
func (l *Ledger) collapseTransactions(block *Block, logging bool) (*collapseResults, error) {
	_collapseState, _ := l.cacheCollapse.LoadOrPut(block.ID, &CollapseState{})
	collapseState := _collapseState.(*CollapseState)

	collapseState.once.Do(func() {
		collapseState.results, collapseState.err = collapseTransactions(l.mempool, block, l.accounts, logging)
	})

	if logging {
		for _, tx := range collapseState.results.applied {
			logEventTX("applied", tx)
		}

		for i, tx := range collapseState.results.rejected {
			logEventTX("rejected", tx, collapseState.results.rejectedErrors[i])
		}
	}

	return collapseState.results, collapseState.err
}

// LogChanges logs all changes made to an AVL tree state snapshot for the purposes
// of logging out changes to account state to Wavelet's HTTP API.
func (l *Ledger) LogChanges(snapshot *avl.Tree, lastRound uint64) {
	balanceLogger := log.Accounts("balance_updated")
	gasBalanceLogger := log.Accounts("gas_balance_updated")
	stakeLogger := log.Accounts("stake_updated")
	rewardLogger := log.Accounts("reward_updated")
	numPagesLogger := log.Accounts("num_pages_updated")

	balanceKey := append(keyAccounts[:], keyAccountBalance[:]...)
	gasBalanceKey := append(keyAccounts[:], keyAccountContractGasBalance[:]...)
	stakeKey := append(keyAccounts[:], keyAccountStake[:]...)
	rewardKey := append(keyAccounts[:], keyAccountReward[:]...)
	numPagesKey := append(keyAccounts[:], keyAccountContractNumPages[:]...)

	var id AccountID

	snapshot.IterateLeafDiff(lastRound, func(key, value []byte) bool {
		switch {
		case bytes.HasPrefix(key, balanceKey):
			copy(id[:], key[len(balanceKey):])

			balanceLogger.Log().
				Hex("account_id", id[:]).
				Uint64("balance", binary.LittleEndian.Uint64(value)).
				Msg("")
		case bytes.HasPrefix(key, gasBalanceKey):
			copy(id[:], key[len(gasBalanceKey):])

			gasBalanceLogger.Log().
				Hex("account_id", id[:]).
				Uint64("gas_balance", binary.LittleEndian.Uint64(value)).
				Msg("")
		case bytes.HasPrefix(key, stakeKey):
			copy(id[:], key[len(stakeKey):])

			stakeLogger.Log().
				Hex("account_id", id[:]).
				Uint64("stake", binary.LittleEndian.Uint64(value)).
				Msg("")
		case bytes.HasPrefix(key, rewardKey):
			copy(id[:], key[len(rewardKey):])

			rewardLogger.Log().
				Hex("account_id", id[:]).
				Uint64("reward", binary.LittleEndian.Uint64(value)).
				Msg("")
		case bytes.HasPrefix(key, numPagesKey):
			copy(id[:], key[len(numPagesKey):])

			numPagesLogger.Log().
				Hex("account_id", id[:]).
				Uint64("num_pages", binary.LittleEndian.Uint64(value)).
				Msg("")
		}

		return true
	})
}

func (l *Ledger) SyncStatus() string {
	l.syncStatusLock.RLock()
	defer l.syncStatusLock.RUnlock()

	switch l.syncStatus {
	case outOfSync:
		return "Node is out of sync"
	case synced:
		return "Node is synced, but not taking part in consensus process yet"
	case fullySynced:
		return "Node is fully synced"
	case finalized:
		return "Node is taking part in consensus process"
	default:
		return "Sync status unknown"
	}
}

func (l *Ledger) isOutOfSync() bool {
	l.syncStatusLock.RLock()
	synced := l.syncStatus == fullySynced
	l.syncStatusLock.RUnlock()

	return !synced
}

func (l *Ledger) applySync(flag bitset) {
	l.syncStatusLock.Lock()
	l.syncStatus |= flag
	l.syncStatusLock.Unlock()
}

func (l *Ledger) setSync(flag bitset) {
	l.syncStatusLock.Lock()
	l.syncStatus = flag
	l.syncStatusLock.Unlock()
}

func (l *Ledger) LastBlockID() [blake2b.Size256]byte {
	// TODO implement this
	return EmptyBlockID
}

func (l *Ledger) LastBlockIndex() uint64 {
	// TODO implement this
	return 0
}
