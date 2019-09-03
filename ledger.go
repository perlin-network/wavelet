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
	"github.com/perlin-network/wavelet/lru"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
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
)

type Ledger struct {
	client  *skademlia.Client
	metrics *Metrics
	indexer *Indexer

	accounts *Accounts
	rounds   *Rounds
	graph    *Graph

	gossiper  *Gossiper
	finalizer *Snowball
	syncer    *Snowball

	consensus sync.WaitGroup

	broadcastNops         bool
	broadcastNopsMaxDepth uint64
	broadcastNopsDelay    time.Time
	broadcastNopsLock     sync.Mutex

	sync      chan struct{}
	syncVotes chan vote

	syncStatus     bitset
	syncStatusLock sync.RWMutex

	cacheCollapse *lru.LRU
	cacheChunks   *lru.LRU

	sendQuota chan struct{}

	stallDetector *StallDetector
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

	if !cfg.GCDisabled {
		go accounts.GC(context.Background())
	}

	rounds, err := NewRounds(kv, sys.PruningLimit)

	var round *Round

	if rounds != nil && err != nil {
		genesis := performInception(accounts.tree, cfg.Genesis)
		if err := accounts.Commit(nil); err != nil {
			logger.Fatal().Err(err).Msg("BUG: accounts.Commit")
		}

		ptr := &genesis

		if _, err := rounds.Save(ptr); err != nil {
			logger.Fatal().Err(err).Msg("BUG: rounds.Save")
		}

		round = ptr
	} else if rounds != nil {
		round = rounds.Latest()
	}

	if round == nil {
		logger.Fatal().Err(err).Msg("BUG: COULD NOT FIND GENESIS, OR STORAGE IS CORRUPTED.")
	}

	graph := NewGraph(WithMetrics(metrics), WithIndexer(indexer), WithRoot(round.End), VerifySignatures())

	gossiper := NewGossiper(context.TODO(), client, metrics)
	finalizer := NewSnowball(WithName("finalizer"), WithBeta(sys.SnowballBeta))
	syncer := NewSnowball(WithName("syncer"), WithBeta(sys.SnowballBeta))

	ledger := &Ledger{
		client:  client,
		metrics: metrics,
		indexer: indexer,

		accounts: accounts,
		rounds:   rounds,
		graph:    graph,

		gossiper:  gossiper,
		finalizer: finalizer,
		syncer:    syncer,

		syncStatus: finalized, // we start node as out of sync, but finalized

		sync:      make(chan struct{}),
		syncVotes: make(chan vote, sys.SnowballK),

		cacheCollapse: lru.NewLRU(16),
		cacheChunks:   lru.NewLRU(1024), // In total, it will take up 1024 * 4MB.

		sendQuota: make(chan struct{}, 2000),
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
	go stallDetector.Run()

	ledger.stallDetector = stallDetector

	ledger.PerformConsensus()

	go ledger.SyncToLatestRound()
	go ledger.PushSendQuota()

	return ledger
}

// AddTransaction adds a transaction to the ledger. If the transaction has
// never been added in the ledgers graph before, it is pushed to the gossip
// mechanism to then be gossiped to this nodes peers. If the transaction is
// invalid or fails any validation checks, an error is returned. No error
// is returned if the transaction has already existed int he ledgers graph
// beforehand.
func (l *Ledger) AddTransaction(tx Transaction) error {
	if l.isOutOfSync() && tx.Sender == l.client.Keys().PublicKey() {
		return ErrOutOfSync
	}

	err := l.graph.AddTransaction(tx)

	if err != nil && errors.Cause(err) != ErrAlreadyExists {
		return err
	}

	if err == nil {
		l.TakeSendQuota()

		l.gossiper.Push(tx)

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

// Graph returns the directed-acyclic-graph of transactions accompanying
// the ledger.
func (l *Ledger) Graph() *Graph {
	return l.graph
}

// Finalizer returns the Snowball finalizer which finalizes the contents of individual
// consensus rounds.
func (l *Ledger) Finalizer() *Snowball {
	return l.finalizer
}

// Rounds returns the round manager for the ledger.
func (l *Ledger) Rounds() *Rounds {
	return l.rounds
}

// PerformConsensus spawns workers related to performing consensus, such as pulling
// missing transactions and incrementally finalizing intervals of transactions in
// the ledgers graph.
func (l *Ledger) PerformConsensus() {
	go l.PullMissingTransactions()
	go l.SyncTransactions()
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
	if balance < sys.TransactionFeeAmount && hex.EncodeToString(publicKey[:]) != sys.FaucetAddress {
		return nil
	}

	nop := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), l.graph.FindEligibleParents()...)

	if err := l.AddTransaction(nop); err != nil {
		return nil
	}

	return l.graph.FindTransaction(nop.ID)
}

func (l *Ledger) SyncTransactions() {
	l.consensus.Add(1)
	defer l.consensus.Done()

	pullingPeriod := 1 * time.Second
	t := time.NewTimer(pullingPeriod)

	for {
		t.Reset(pullingPeriod)

		select {
		case <-l.sync:
			return
		case <-t.C:
		}

		l.syncStatusLock.RLock()
		status := l.syncStatus
		l.syncStatusLock.RUnlock()
		if status != synced {
			continue
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
			default:
			}

			continue
		}

		rand.Shuffle(len(peers), func(i, j int) {
			peers[i], peers[j] = peers[j], peers[i]
		})
		now := time.Now()
		currentDepth := l.graph.RootDepth()
		lowLimit := currentDepth - sys.MaxDepthDiff
		highLimit := currentDepth + sys.MaxDownloadDepthDiff
		txs := l.Graph().GetTransactionsByDepth(&lowLimit, &highLimit)

		ids := make([][]byte, len(txs))
		for i, tx := range txs {
			ids[i] = tx.ID[:]
		}

		req := &DownloadTxRequest{
			Depth:   currentDepth,
			SkipIds: ids,
		}

		conn := peers[0]
		client := NewWaveletClient(conn)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		batch, err := client.DownloadTx(ctx, req)
		if err != nil {
			fmt.Println("failed to download missing transactions:", err)
			cancel()
			continue
		}
		cancel()

		txCount, logicalCount := 0, 0
		for _, buf := range batch.Transactions {
			tx, err := UnmarshalTransaction(bytes.NewReader(buf))
			if err != nil {
				fmt.Printf("error unmarshaling downloaded tx [%v]: %+v", err, tx)
				continue
			}

			if err := l.AddTransaction(tx); err != nil && errors.Cause(err) != ErrMissingParents {
				fmt.Printf("error adding synced tx to graph [%v]: %+v\n", err, tx)
				continue
			}

			logicalCount += tx.LogicalUnits()
			txCount++
		}

		if txCount > 0 {
			logger := log.Consensus("transaction-sync")
			logger.Debug().
				Int("synced_tx", txCount).
				Int("logical_tx", logicalCount).
				Dur("duration", time.Now().Sub(now)).
				Msg("Transaction downloaded")
		}
	}
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
		select {
		case <-l.sync:
			return
		default:
		}

		missing := l.graph.Missing()

		if len(missing) == 0 {
			select {
			case <-l.sync:
				return
			case <-time.After(100 * time.Millisecond):
			}

			continue
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

		rand.Shuffle(len(missing), func(i, j int) {
			missing[i], missing[j] = missing[j], missing[i]
		})
		if len(missing) > 256 {
			missing = missing[:256]
		}

		req := &DownloadMissingTxRequest{Ids: make([][]byte, len(missing))}

		for i, id := range missing {
			req.Ids[i] = id[:]
		}

		conn := peers[0]
		client := NewWaveletClient(conn)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		batch, err := client.DownloadMissingTx(ctx, req)
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
				fmt.Printf("error unmarshaling downloaded tx [%v]: %+v\n", err, tx)
				continue
			}

			if err := l.AddTransaction(tx); err != nil && errors.Cause(err) != ErrMissingParents {
				fmt.Printf("error adding downloaded tx to graph [%v]: %+v\n", err, tx)
				continue
			}

			count += int64(tx.LogicalUnits())
		}

		l.metrics.downloadedTX.Mark(count)
		l.metrics.receivedTX.Mark(count)
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

	for {
		l.metrics.finalizationLatency.Time(l.finalize)
	}
}

func (l *Ledger) finalize() {
	select {
	case <-l.sync:
		return
	default:
	}

	if len(l.client.ClosestPeers()) < sys.SnowballK {
		select {
		case <-l.sync:
			return
		case <-time.After(1 * time.Second):
		}

		return
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

				return
			}

			select {
			case <-l.sync:
				return
			case <-time.After(1 * time.Millisecond):
			}

			return
		}

		results, err := l.collapseTransactions(current.Index+1, current.End, *eligible, false)
		if err != nil {
			fmt.Println("error collapsing transactions during finalization", err)
			return
		}

		candidate := NewRound(current.Index+1, results.snapshot.Checksum(), uint64(results.appliedCount), current.End, *eligible)
		l.finalizer.Prefer(&candidate)

		return
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

	voteChan := make(chan vote, sys.SnowballK)
	go CollectVotes(l.accounts, l.finalizer, voteChan, &workerWG, sys.SnowballK)

	for i := 0; i < cap(workerChan); i++ {
		go func() {
			for conn := range workerChan {
				l.metrics.queryLatency.Time(func() {
					l.query(conn, voteChan, current, currentDifficulty)
				})
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
		peers, err := SelectPeers(l.client.ClosestPeers(), sys.SnowballK)
		if err != nil {
			close(workerChan)
			workerWG.Wait()
			workerWG.Add(1)
			close(voteChan)
			workerWG.Wait() // Wait for vote processor worker to stop

			return
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
			fmt.Println("error collapsing transactions during finalization:", err)
		}
		return
	}

	if uint64(results.appliedCount) != preferred.Applied {
		fmt.Printf("Expected to have applied %d transactions finalizing a round, but only applied %d transactions instead.\n", preferred.Applied, results.appliedCount)
		return
	}

	if results.snapshot.Checksum() != preferred.Merkle {
		fmt.Printf("Expected preferred round's merkle root to be %x, but got %x.\n", preferred.Merkle, results.snapshot.Checksum())
		return
	}

	pruned, err := l.rounds.Save(preferred)
	if err != nil {
		fmt.Printf("Failed to save preferred round to our database: %v\n", err)
	}

	if pruned != nil {
		count := l.graph.PruneBelowDepth(pruned.End.Depth)

		logger := log.Consensus("prune")
		logger.Debug().
			Int("num_tx", count).
			Uint64("current_round_id", preferred.Index).
			Uint64("pruned_round_id", pruned.Index).
			Msg("Pruned away round and transactions.")
	}

	l.graph.UpdateRootDepth(preferred.End.Depth)

	if err = l.accounts.Commit(results.snapshot); err != nil {
		fmt.Printf("Failed to commit collaped state to our database: %v\n", err)
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

	//go ExportGraphDOT(finalized, contractIDs.graph)
}

func (l *Ledger) query(conn *grpc.ClientConn, voteChan chan<- vote, current *Round, currentDifficulty byte) {
	client := NewWaveletClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)

	p := &peer.Peer{}

	req := &QueryRequest{RoundIndex: current.Index + 1}

	res, err := client.Query(ctx, req, grpc.Peer(p))
	if err != nil {
		cancel()
		fmt.Println("error while querying peer: ", err)
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
		voteChan <- vote{voter: voter, value: ZeroRoundPtr}
		return
	}

	if round.ID == ZeroRoundID || round.Start.ID == ZeroTransactionID || round.End.ID == ZeroTransactionID {
		voteChan <- vote{voter: voter, value: ZeroRoundPtr}
		return
	}

	if round.End.Depth <= round.Start.Depth {
		return
	}

	if round.Index != current.Index+1 {
		if round.Index > sys.SyncIfRoundsDifferBy+current.Index {
			select {
			case l.syncVotes <- vote{voter: voter, value: &round}:
			default:
			}
		}

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
			fmt.Println("error collapsing transactions:", err)
		}
		return
	}

	if uint64(results.appliedCount) != round.Applied {
		fmt.Printf("applied %d but expected %d, rejected = %d, ignored = %d\n", results.appliedCount, round.Applied, results.rejectedCount, results.ignoredCount)
		return
	}

	if results.snapshot.Checksum() != round.Merkle {
		fmt.Printf("got merkle %x but expected %x\n", results.snapshot.Checksum(), round.Merkle)
		return
	}

	voteChan <- vote{voter: voter, value: &round}
}

type outOfSyncVote struct {
	outOfSync bool
}

func (o *outOfSyncVote) GetID() string {
	return fmt.Sprintf("%v", o.outOfSync)
}

func (l *Ledger) SyncToLatestRound() {
	voteWG := new(sync.WaitGroup)

	go CollectVotes(l.accounts, l.syncer, l.syncVotes, voteWG, sys.SnowballK)

	syncTimeoutMultiplier := 0
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
					ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)

					p := &peer.Peer{}

					res, err := client.CheckOutOfSync(
						ctx,
						&OutOfSyncRequest{RoundIndex: current.Index},
						grpc.Peer(p),
					)
					if err != nil {
						fmt.Println("error while checking out of sync", err)
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

					l.syncVotes <- vote{voter: voter, value: &outOfSyncVote{outOfSync: res.OutOfSync}}

					wg.Done()
				}()
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
			time.Sleep(time.Duration(syncTimeoutMultiplier) * time.Second)

			continue
		}

		l.setSync(outOfSync)

		syncTimeoutMultiplier = 0

		shutdown := func() {
			close(l.sync)
			l.consensus.Wait() // Wait for all consensus-related workers to shutdown.

			voteWG.Add(1)
			close(l.syncVotes)
			voteWG.Wait() // Wait for the vote processor worker to shutdown.

			l.finalizer.Reset() // Reset consensus Snowball sampler.
			l.syncer.Reset()    // Reset syncing Snowball sampler.
		}

		restart := func() { // Respawn all previously stopped workers.
			l.syncVotes = make(chan vote, sys.SnowballK)
			go CollectVotes(l.accounts, l.syncer, l.syncVotes, voteWG, sys.SnowballK)

			l.sync = make(chan struct{})
			l.PerformConsensus()
		}

		shutdown() // Shutdown all consensus-related workers.

		logger := log.Sync("syncing")
		logger.Info().
			Uint64("current_round", current.Index).
			Msg("Noticed that we are out of sync; downloading latest state Snapshot from our peer(s).")

	SYNC:

		conns, err := SelectPeers(l.client.ClosestPeers(), sys.SnowballK)
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

		chunks := make([][]byte, len(sources))

		// Streams may not concurrently send and receive messages at once.

		streamLocks := make(map[Wavelet_SyncClient]*sync.Mutex)
		var streamLock sync.Mutex

		workers := make(chan source, 16)

		var workerWG sync.WaitGroup
		workerWG.Add(cap(workers))

		var chunkWG sync.WaitGroup
		chunkWG.Add(len(chunks))

		logger.Debug().
			Int("num_chunks", len(sources)).
			Int("num_workers", cap(workers)).
			Msg("Starting up workers to downloaded all chunks of data needed to sync to the latest round...")

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

						if len(chunk) > sys.SyncChunkSize {
							continue
						}

						if blake2b.Sum256(chunk[:]) != src.checksum {
							continue
						}

						// We found the chunk! Store the chunks contents.

						chunks[src.idx] = chunk
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

		var diff []byte

		for i, chunk := range chunks {
			if chunk == nil {
				logger.Error().
					Uint64("target_round", latest.Index).
					Hex("chunk_checksum", sources[i].checksum[:]).
					Msg("Could not download one of the chunks necessary to sync to the latest round! Retrying...")

				goto SYNC
			}

			diff = append(diff, chunk...)
		}

		logger.Info().
			Int("num_chunks", len(sources)).
			Uint64("target_round", latest.Index).
			Msg("All chunks have been successfully verified and re-assembled into a diff. Applying diff...")

		snapshot := l.accounts.Snapshot()

		if err := snapshot.ApplyDiff(diff); err != nil {
			logger.Error().
				Uint64("target_round", latest.Index).
				Err(err).
				Msg("Failed to apply re-assembled diff to our ledger state. Restarting sync...")
			goto SYNC
		}

		if checksum := snapshot.Checksum(); checksum != latest.Merkle {
			logger.Error().
				Uint64("target_round", latest.Index).
				Hex("expected_merkle_root", latest.Merkle[:]).
				Hex("yielded_merkle_root", checksum[:]).
				Msg("Failed to apply re-assembled diff to our ledger state. Restarting sync...")

			goto SYNC
		}

		pruned, err := l.rounds.Save(latest)
		if err != nil {
			fmt.Printf("Failed to save finalized round to our database: %v\n", err)
			goto SYNC
		}

		if pruned != nil {
			count := l.graph.PruneBelowDepth(pruned.End.Depth)

			logger := log.Consensus("prune")
			logger.Debug().
				Int("num_tx", count).
				Uint64("current_round_id", latest.Index).
				Uint64("pruned_round_id", pruned.Index).
				Msg("Pruned away round and transactions.")
		}

		l.graph.UpdateRoot(latest.End)

		if err := l.accounts.Commit(snapshot); err != nil {
			logger := log.Node()
			logger.Fatal().Err(err).Msg("failed to commit collapsed state to our database")
		}

		logger = log.Sync("apply")
		logger.Info().
			Int("num_chunks", len(chunks)).
			Uint64("old_round", current.Index).
			Uint64("new_round", latest.Index).
			Uint8("old_difficulty", current.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
			Uint8("new_difficulty", latest.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
			Hex("new_root", latest.End.ID[:]).
			Hex("old_root", current.End.ID[:]).
			Hex("new_merkle_root", latest.Merkle[:]).
			Hex("old_merkle_root", current.Merkle[:]).
			Msg("Successfully built a new state Snapshot out of chunk(s) we have received from peers.")

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
func (l *Ledger) collapseTransactions(round uint64, start, end Transaction, logging bool) (*collapseResults, error) {
	_collapseState, _ := l.cacheCollapse.LoadOrPut(end.ID, &CollapseState{})
	collapseState := _collapseState.(*CollapseState)

	collapseState.once.Do(func() {
		t := time.Now()
		collapseState.results, collapseState.err = collapseTransactions(l.graph, l.accounts, round, l.Rounds().Latest(), start, end, logging)
		l.metrics.collapseLatency.Update(time.Now().Sub(t))
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
