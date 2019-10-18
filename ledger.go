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
	"math/big"
	"math/rand"
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
	ErrOutOfSync     = errors.New("Node is currently ouf of sync. Please try again later.")
	ErrAlreadyExists = errors.New("transaction already exists in the graph")
	ErrMissingTx     = errors.New("missing transaction")

	EmptyBlockID BlockID
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

	finalizer := NewSnowball()
	syncer := NewSnowball()

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

	go ledger.SyncToLatestBlock()
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
func (l *Ledger) AddTransaction(txs ...Transaction) error {
	l.mempool.Add(l.blocks.Latest().ID, txs...)

	l.TakeSendQuota()

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
// blocks.
func (l *Ledger) Finalizer() *Snowball {
	return l.finalizer
}

// Blocks returns the block manager for the ledger.
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
	go l.FinalizeBlocks()
}

func (l *Ledger) Snapshot() *avl.Tree {
	return l.accounts.Snapshot()
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

		case <-time.After(1 * time.Second):
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

		// Marshal the transaction ids
		buf := new(bytes.Buffer)
		if _, err := l.mempool.WriteTransactionIDs(buf); err != nil {
			logger.Error().Err(err).Msg("failed to marshal transaction ids")
			continue
		}

		req := &TransactionPullRequest{Filter: buf.Bytes()}

		conn := peers[0]
		client := NewWaveletClient(conn)

		ctx, cancel := context.WithTimeout(context.Background(), conf.GetDownloadTxTimeout())
		batch, err := client.PullTransactions(ctx, req)
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

			if err := l.AddTransaction(tx); err != nil {
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

// FinalizeBlocks continuously attempts to finalize blocks.
func (l *Ledger) FinalizeBlocks() {
	for {
		preferred := l.finalizer.Preferred()
		decided := l.finalizer.Decided()

		if preferred == nil {
			proposedBlock := l.proposeBlock()

			if proposedBlock != nil {
				l.finalizer.Prefer(newPreferredBlockVote(proposedBlock))
			}
		} else {
			if decided {
				l.finalize(*preferred.val.(*Block))
			} else {
				l.query()
			}
		}
	}
}

// proposeBlock takes all transactions from the first quarter of the mempool
// and creates a new block, which will be proposed to be finalized as the
// next block in the chain.
func (l *Ledger) proposeBlock() *Block {
	maxIndex := (&big.Int{}).Exp(big.NewInt(2), big.NewInt(256), nil)
	maxIndex = maxIndex.Div(maxIndex, big.NewInt(4))

	proposing := make([]TransactionID, 0)
	l.mempool.AscendLessThan(maxIndex, func(tx Transaction) bool {
		proposing = append(proposing, tx.ID)
		return true
	})

	if len(proposing) == 0 {
		return nil
	}

	// TODO(kenta): derive the merkle root after applying all transactions to be proposed in the block, and incorporate
	//	the merkle root into NewBlock()

	proposed := NewBlock(l.blocks.Latest().Index+1, l.accounts.tree.Checksum(), proposing...)
	return &proposed
}

func (l *Ledger) finalize(block Block) {
	current := l.blocks.Latest()

	l.mempool.Reshuffle(*current, block)

	results, err := l.collapseTransactions(&block, false)
	if err != nil {
		logger := log.Node()
		logger.Error().
			Err(err).
			Msg("error collapsing transactions during finalization")

		return
	}

	if results.appliedCount+results.rejectedCount != len(block.Transactions) {
		logger := log.Node()
		logger.Error().
			Err(err).
			Int("expected", len(block.Transactions)).
			Int("actual", results.appliedCount).
			Msg("Number of applied transactions does not match")

		return
	}

	pruned, err := l.blocks.Save(&block)
	if err != nil {
		logger := log.Node()
		logger.Error().
			Err(err).
			Msg("Failed to save preferred block to database")

		return
	}

	if pruned != nil {
		count := l.mempool.Prune(*pruned)

		logger := log.Consensus("prune")
		logger.Debug().
			Int("num_tx", count).
			Uint64("current_block", block.Index).
			Uint64("pruned_block", pruned.Index).
			Msg("Pruned away block and transactions.")
	}

	if err = l.accounts.Commit(results.snapshot); err != nil {
		logger := log.Node()
		logger.Error().
			Err(err).
			Msg("Failed to commit collaped state to our database")

		return
	}

	l.metrics.acceptedTX.Mark(int64(results.appliedCount))

	l.LogChanges(results.snapshot, current.Index)

	l.applySync(finalized)

	// Reset snowball
	l.finalizer.Reset()

	logger := log.Consensus("finalized")
	logger.Info().
		Int("num_applied_tx", results.appliedCount).
		Int("num_rejected_tx", results.rejectedCount).
		Int("num_ignored_tx", results.ignoredCount).
		Uint64("old_block", current.Index).
		Uint64("new_block", block.Index).
		Msg("Finalized consensus block, and initialized a new block.")
}

func (l *Ledger) query() {
	if len(l.client.ClosestPeerIDs()) < conf.GetSnowballK() {
		// TODO not enough peers, what do to ?
		return
	}

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

					if block.ID == ZeroBlockID {
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
	for _, vote := range votes {
		if vote.block == nil {
			continue
		}

		if vote.block.Index != l.blocks.Latest().Index {
			vote.block = nil
			continue
		}

		if vote.block.ID == ZeroBlockID {
			continue
		}

		for _, id := range vote.block.Transactions {
			if tx := l.mempool.Find(id); tx == nil {
				vote.block.ID = ZeroBlockID
				break
			}
		}
	}

	TickForFinalization(l.accounts, l.finalizer, votes)
}

type outOfSyncVote struct {
	outOfSync bool
}

func (o *outOfSyncVote) GetID() string {
	return fmt.Sprintf("%v", o.outOfSync)
}

// SyncToLatestBlock continuously checks if the node is out of sync from its peers.
// If the majority of its peers responded that it is out of sync (decided using snowball),
// the node will attempt to sync its state to the latest block by downloading the AVL tree
// diff from its peers and applying the diff to its local AVL tree.
func (l *Ledger) SyncToLatestBlock() {
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

			current := l.blocks.Latest()

			var wg sync.WaitGroup
			wg.Add(len(conns))

			for _, conn := range conns {
				go func(conn *grpc.ClientConn) {
					client := NewWaveletClient(conn)

					ctx, cancel := context.WithTimeout(context.Background(), conf.GetCheckOutOfSyncTimeout())

					p := &peer.Peer{}

					res, err := client.CheckOutOfSync(
						ctx,
						&OutOfSyncRequest{BlockIndex: current.Index},
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

		current := l.blocks.Latest()
		preferred := l.syncer.Preferred()

		oos := preferred.val.(*outOfSyncVote).outOfSync
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
			Uint64("current_block_index", current.Index).
			Msg("Noticed that we are out of sync; downloading latest state Snapshot from our peer(s).")

	SYNC:
		conns, err := SelectPeers(l.client.ClosestPeers(), conf.GetSnowballK())
		if err != nil {
			logger.Warn().Msg("It looks like there are no peers for us to sync with. Retrying...")

			time.Sleep(1 * time.Second)

			goto SYNC
		}

		req := &SyncRequest{Data: &SyncRequest_BlockId{BlockId: current.Index}}

		type response struct {
			header *SyncInfo
			latest Block
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

			latest, err := UnmarshalBlock(bytes.NewReader(header.Block))
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

		// Select a block to sync to which the majority of peers are on.

		var latest *Block
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
			logger.Warn().Msg("It looks like our peers could not decide on what the latest block currently is. Retrying...")

			dispose()
			goto SYNC
		}

		logger.Debug().
			Uint64("block_id", latest.Index).
			Hex("merkle_root", latest.Merkle[:]).
			Msg("Discovered the latest block the majority of our peers ar eon.")

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
			Msg("Starting up workers to downloaded all chunks of data needed to sync to the latest block...")

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
			Msg("Downloaded whatever chunks were available to sync to the latest block, and shutted down all workers. Checking validity of chunks...")

		dispose() // Shutdown all streams as we no longer need them.

		// Check all chunks has been received
		var diffSize int64
		for i, src := range sources {
			if src.size == 0 {
				logger.Error().
					Uint64("target_block_id", latest.Index).
					Hex("chunk_checksum", sources[i].checksum[:]).
					Msg("Could not download one of the chunks necessary to sync to the latest block! Retrying...")

				cleanup()
				goto SYNC
			}

			diffSize += int64(src.size)
		}

		if _, err := io.CopyN(diffBuffer, chunksBuffer, diffSize); err != nil {
			logger.Error().
				Uint64("target_block_id", latest.Index).
				Err(err).
				Msg("Failed to write chunks to bounded memory buffer. Restarting sync...")

			cleanup()
			goto SYNC
		}

		logger.Info().
			Int("num_chunks", len(sources)).
			Uint64("target_block", latest.Index).
			Msg("All chunks have been successfully verified and re-assembled into a diff. Applying diff...")

		snapshot := l.accounts.Snapshot()
		if err := snapshot.ApplyDiff(diffBuffer); err != nil {
			logger.Error().
				Uint64("target_block_id", latest.Index).
				Err(err).
				Msg("Failed to apply re-assembled diff to our ledger state. Restarting sync...")

			cleanup()
			goto SYNC
		}

		if checksum := snapshot.Checksum(); checksum != latest.Merkle {
			logger.Error().
				Uint64("target_block_id", latest.Index).
				Hex("expected_merkle_root", latest.Merkle[:]).
				Hex("yielded_merkle_root", checksum[:]).
				Msg("Failed to apply re-assembled diff to our ledger state. Restarting sync...")

			cleanup()
			goto SYNC
		}

		pruned, err := l.blocks.Save(latest)
		if err != nil {
			logger.Error().
				Err(err).
				Msg("Failed to save finalized block to our database")

			cleanup()
			goto SYNC
		}

		if pruned != nil {
			count := l.mempool.Prune(*pruned)

			logger := log.Consensus("prune")
			logger.Debug().
				Int("num_tx", count).
				Uint64("current_block", latest.Index).
				Uint64("pruned_block", pruned.Index).
				Msg("Pruned away block and transactions.")
		}

		if err := l.accounts.Commit(snapshot); err != nil {
			cleanup()
			logger := log.Node()
			logger.Fatal().Err(err).Msg("failed to commit collapsed state to our database")
		}

		logger = log.Sync("apply")
		logger.Info().
			Int("num_chunks", len(sources)).
			Uint64("old_block_index", current.Index).
			Uint64("new_block_index", latest.Index).
			Hex("new_block_id", latest.ID[:]).
			Hex("old_block_id", current.ID[:]).
			Hex("new_merkle_root", latest.Merkle[:]).
			Hex("old_merkle_root", current.Merkle[:]).
			Msg("Successfully built a new state snapshot out of chunk(s) we have received from peers.")

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
func (l *Ledger) LogChanges(snapshot *avl.Tree, lastBlockIndex uint64) {
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

	snapshot.IterateLeafDiff(lastBlockIndex, func(key, value []byte) bool {
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

func (l *Ledger) Mempool() *Mempool {
	return l.mempool
}
