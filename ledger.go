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
	"io"
	"math/rand"
	"sync"
	"time"

	cuckoo "github.com/seiflotfy/cuckoofilter"

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
	"google.golang.org/grpc/peer"
)

type bitset uint8

const (
	outOfSync   bitset = 0 //nolint:staticcheck
	synced             = 1
	finalized          = 2
	fullySynced        = 3
)

var (
	ErrMissingTx = errors.New("missing transaction")
)

type Ledger struct {
	client  *skademlia.Client
	metrics *Metrics
	indexer *Indexer

	accounts     *Accounts
	blocks       *Blocks
	transactions *Transactions

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

	stopWG   sync.WaitGroup
	cancelGC context.CancelFunc

	transactionsSyncIndexLock sync.RWMutex
	transactionsSyncIndex     *cuckoo.Filter

	queryBlockCache map[[blake2b.Size256]byte]*Block

	queueWorkerPool *WorkerPool
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
			logger.Fatal().Err(err).Msg("BUG: blocks.Save")
		}

		block = ptr
	} else if blocks != nil {
		block = blocks.Latest()
	}

	if block == nil {
		logger.Fatal().Err(err).Msg("BUG: COULD NOT FIND GENESIS, OR STORAGE IS CORRUPTED.")
		return nil
	}

	transactions := NewTransactions(block.Index)

	finalizer := NewSnowball()
	syncer := NewSnowball()

	ledger := &Ledger{
		client:  client,
		metrics: metrics,
		indexer: indexer,

		accounts:     accounts,
		blocks:       blocks,
		transactions: transactions,

		finalizer: finalizer,
		syncer:    syncer,

		syncStatus: finalized, // we start node as out of sync, but finalized

		sync: make(chan struct{}),

		cacheCollapse: lru.NewLRU(16),
		fileBuffers:   newFileBufferPool(sys.SyncPooledFileSize, ""),

		sendQuota: make(chan struct{}, 2000),

		transactionsSyncIndex: cuckoo.NewFilter(conf.GetBloomFilterM()),

		queryBlockCache: make(map[BlockID]*Block),

		queueWorkerPool: NewWorkerPool(),
	}

	if !cfg.GCDisabled {
		ctx, cancel := context.WithCancel(context.Background())
		ledger.stopWG.Add(1)
		go accounts.GC(ctx, &ledger.stopWG)

		ledger.cancelGC = cancel
	}

	stallDetector := NewStallDetector(StallDetectorConfig{
		MaxMemoryMB: cfg.MaxMemoryMB,
	}, StallDetectorDelegate{
		PrepareShutdown: func(err error) {
			logger := log.Node()
			logger.Error().Err(err).Msg("Shutting down node...")
		},
	})

	ledger.stopWG.Add(1)
	go stallDetector.Run(&ledger.stopWG)

	ledger.stallDetector = stallDetector

	ledger.queueWorkerPool.Start(16)

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

	if l.cancelGC != nil {
		l.cancelGC()
	}

	l.queueWorkerPool.Stop()

	l.stallDetector.Stop()

	l.stopWG.Wait()
}

// AddTransaction adds a transaction to the ledger and adds it's id to bloom filter used to sync transactions.
func (l *Ledger) AddTransaction(txs ...Transaction) {
	l.transactions.BatchAdd(l.blocks.Latest().ID, txs...)

	l.transactionsSyncIndexLock.Lock()
	for _, tx := range txs {
		l.transactionsSyncIndex.InsertUnique(tx.ID[:])
	}
	l.transactionsSyncIndexLock.Unlock()

	//l.TakeSendQuota()
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

// Transactions returns the transaction manager for the ledger.
func (l *Ledger) Transactions() *Transactions {
	return l.transactions
}

// Restart restart wavelet process by means of stall detector (approach is platform dependent)
func (l *Ledger) Restart() error {
	return l.stallDetector.tryRestart()
}

// PerformConsensus spawns workers related to performing consensus, such as pulling
// missing transactions and incrementally finalizing intervals of transactions in
// the ledgers graph.
func (l *Ledger) PerformConsensus() {
	l.consensus.Add(1)
	go l.PullMissingTransactions()

	l.consensus.Add(1)
	go l.SyncTransactions()

	l.consensus.Add(1)
	go l.FinalizeBlocks()
}

func (l *Ledger) Snapshot() *avl.Tree {
	return l.accounts.Snapshot()
}

// SyncTransactions is an infinite loop which constantly sends transaction ids from its index
// in form of the bloom filter to randomly sampled number of peers and adds to it's state all received
// transactions.
func (l *Ledger) SyncTransactions() { // nolint:gocognit
	defer l.consensus.Done()

	for {
		select {
		case <-l.sync:
			return
		case <-time.After(3 * time.Second):
		}

		snowballK := conf.GetSnowballK()

		peers, err := SelectPeers(l.client.ClosestPeers(), snowballK)
		if err != nil {
			continue
		}

		logger := log.Sync("sync_tx")

		l.transactionsSyncIndexLock.RLock()
		cf := l.transactionsSyncIndex.Encode()
		l.transactionsSyncIndexLock.RUnlock()

		if err != nil {
			logger.Error().Err(err).Msg("failed to marshal bloom filter")
			continue
		}

		var wg sync.WaitGroup

		bfReq := &TransactionsSyncRequest{
			Data: &TransactionsSyncRequest_Filter{
				Filter: cf,
			},
		}

		for _, p := range peers {
			wg.Add(1)
			go func(conn *grpc.ClientConn) {
				defer wg.Done()

				ctx, cancel := context.WithTimeout(context.Background(), conf.GetDownloadTxTimeout())
				defer cancel()

				stream, err := NewWaveletClient(conn).SyncTransactions(ctx)
				if err != nil {
					logger.Error().Err(err).Msg("failed to create sync transactions stream")
					return
				}

				defer func() {
					if err := stream.CloseSend(); err != nil {
						logger.Error().Err(err).Msg("failed to send sync transactions bloom filter")
					}
				}()

				if err := stream.Send(bfReq); err != nil {
					logger.Error().Err(err).Msg("failed to send sync transactions bloom filter")
					return
				}

				res, err := stream.Recv()
				if err != nil {
					logger.Error().Err(err).Msg("failed to receive sync transactions header")
					return
				}

				count := res.GetTransactionsNum()
				if count == 0 {
					return
				}

				if count > conf.GetTXSyncLimit() {
					logger.Debug().
						Uint64("count", count).
						Str("peer_address", conn.Target()).
						Msg("Bad number of transactions would be received")
					return
				}

				logger.Debug().
					Uint64("count", count).
					Msg("Requesting transaction(s) to sync.")

				for count > 0 {
					req := TransactionsSyncRequest_ChunkSize{
						ChunkSize: conf.GetTXSyncChunkSize(),
					}

					if count < req.ChunkSize {
						req.ChunkSize = count
					}

					if err := stream.Send(&TransactionsSyncRequest{Data: &req}); err != nil {
						logger.Error().Err(err).Msg("failed to receive sync transactions header")
						return
					}

					res, err := stream.Recv()
					if err != nil {
						logger.Error().Err(err).Msg("failed to receive sync transactions header")
						return
					}

					txResponse := res.GetTransactions()
					if txResponse == nil {
						return
					}

					transactions := make([]Transaction, 0, len(txResponse.Transactions))
					for _, txBody := range txResponse.Transactions {
						tx, err := UnmarshalTransaction(bytes.NewReader(txBody))
						if err != nil {
							logger.Error().Err(err).Msg("failed to unmarshal synced transaction")
							continue
						}

						transactions = append(transactions, tx)
					}

					downloadedNum := len(transactions)
					count -= uint64(downloadedNum)

					l.AddTransaction(transactions...)

					l.metrics.downloadedTX.Mark(int64(downloadedNum))
					l.metrics.receivedTX.Mark(int64(downloadedNum))
				}
			}(p.Conn())
		}

		wg.Wait()
	}
}

// PullMissingTransactions is a goroutine which continuously pulls missing transactions
// from randomly sampled peers in the network. It does so by sending a list of
// transaction IDs which node is missing.
func (l *Ledger) PullMissingTransactions() {
	defer l.consensus.Done()

	for {
		select {
		case <-l.sync:
			return
		case <-time.After(100 * time.Millisecond):
		}

		snowballK := conf.GetSnowballK()

		peers, err := SelectPeers(l.client.ClosestPeers(), snowballK)
		if err != nil {
			continue
		}

		logger := log.Sync("pull_missing_tx")

		// Build list of transaction IDs
		missingIDs := l.transactions.MissingIDs()
		req := &TransactionPullRequest{TransactionIds: make([][]byte, 0, len(missingIDs))}
		for _, txID := range missingIDs {
			txID := txID
			req.TransactionIds = append(req.TransactionIds, txID[:])
		}

		responseChan := make(chan *TransactionPullResponse)
		for _, p := range peers {
			go func(conn *grpc.ClientConn) {
				client := NewWaveletClient(conn)

				ctx, cancel := context.WithTimeout(context.Background(), conf.GetDownloadTxTimeout())
				defer cancel()

				batch, err := client.PullTransactions(ctx, req)
				if err != nil {
					logger.Error().Err(err).Msg("failed to download missing transactions")

					responseChan <- nil

					return
				}

				responseChan <- batch
			}(p.Conn())
		}

		responses := make([]*TransactionPullResponse, 0, len(peers))
		for i := 0; i < cap(responses); i++ {
			response := <-responseChan

			if response == nil {
				continue
			}

			responses = append(responses, response)
		}

		close(responseChan)

		count := int64(0)

		pulled := map[TransactionID]Transaction{}

		for _, res := range responses {
			for _, buf := range res.Transactions {
				tx, err := UnmarshalTransaction(bytes.NewReader(buf))
				if err != nil {
					logger.Error().
						Err(err).
						Hex("tx_id", tx.ID[:]).
						Msg("error unmarshaling downloaded tx")
					continue
				}

				if _, ok := pulled[tx.ID]; !ok {
					pulled[tx.ID] = tx
					count += int64(tx.LogicalUnits())
				}
			}
		}

		pulledTXs := make([]Transaction, 0, len(pulled))
		for _, tx := range pulled {
			pulledTXs = append(pulledTXs, tx)
		}

		l.AddTransaction(pulledTXs...)

		if count > 0 {
			logger.Info().
				Int64("count", count).
				Msg("Pulled missing transaction(s).")
		}

		l.metrics.downloadedTX.Mark(count)
		l.metrics.receivedTX.Mark(count)
	}
}

// FinalizeBlocks continuously attempts to finalize blocks.
func (l *Ledger) FinalizeBlocks() {
	defer l.consensus.Done()

	for {
		select {
		case <-l.sync:
			return

		default:
		}

		preferred := l.finalizer.Preferred()
		decided := l.finalizer.Decided()

		if preferred == nil {
			proposedBlock := l.proposeBlock()

			logger := log.Consensus("proposal")
			if proposedBlock != nil {
				logger.Debug().
					Hex("block_id", proposedBlock.ID[:]).
					Uint64("block_index", proposedBlock.Index).
					Int("num_transactions", len(proposedBlock.Transactions)).
					Msg("Proposing block...")

				l.finalizer.Prefer(&finalizationVote{
					block: proposedBlock,
				})
			}
		} else {
			if decided {
				l.finalize(*preferred.Value().(*Block))
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
	proposing := l.transactions.ProposableIDs()

	if len(proposing) == 0 {
		return nil
	}

	logger := log.Node()

	block, err := NewBlock(l.blocks.Latest().Index+1, l.accounts.tree.Checksum(), proposing...)
	if err != nil {
		logger.Error().
			Err(err).
			Msg("error creating new block")

		return nil
	}

	proposed := block

	results, err := l.collapseTransactions(&proposed, false)
	if err != nil {
		logger.Error().
			Err(err).
			Msg("error collapsing transactions during block proposal")

		return nil
	}

	proposed.Merkle = results.snapshot.Checksum()

	return &proposed
}

func (l *Ledger) finalize(block Block) {
	current := l.blocks.Latest()

	logger := log.Consensus("finalized")

	results, err := l.collapseTransactions(&block, true)
	if err != nil {
		logger := log.Node()
		logger.Error().
			Err(err).
			Msg("error collapsing transactions during finalization")

		return
	}

	if checksum := results.snapshot.Checksum(); checksum != block.Merkle {
		logger := log.Node()
		logger.Error().
			Uint64("target_block_id", block.Index).
			Hex("expected_merkle_root", block.Merkle[:]).
			Hex("yielded_merkle_root", checksum[:]).
			Msg("Merkle root does not match")

		return
	}

	pruned := l.transactions.ReshufflePending(block)
	l.transactionsSyncIndexLock.Lock()
	for _, txID := range pruned {
		l.transactionsSyncIndex.Delete(txID[:])
	}
	l.transactionsSyncIndexLock.Unlock()

	_, err = l.blocks.Save(&block)
	if err != nil {
		logger := log.Node()
		logger.Error().
			Err(err).
			Msg("Failed to save preferred block to database")

		return
	}

	if err = l.accounts.Commit(results.snapshot); err != nil {
		logger := log.Node()
		logger.Error().
			Err(err).
			Msg("Failed to commit collaped state to our database")

		return
	}

	l.metrics.acceptedTX.Mark(int64(results.appliedCount))
	l.metrics.finalizedBlocks.Mark(1)

	l.LogChanges(results.snapshot, current.Index)

	l.applySync(finalized)

	// Reset snowball
	l.finalizer.Reset()

	logger.Info().
		Int("num_applied_tx", results.appliedCount).
		Int("num_rejected_tx", results.rejectedCount).
		Int("num_pruned_tx", len(pruned)).
		Uint64("old_block_height", current.Index).
		Uint64("new_block_height", block.Index).
		Hex("old_block_id", current.ID[:]).
		Hex("new_block_id", block.ID[:]).
		Msg("Finalized block.")
}

func (l *Ledger) query() {
	snowballK := conf.GetSnowballK()

	peers, err := SelectPeers(l.client.ClosestPeers(), snowballK)
	if err != nil {
		return
	}

	current := l.blocks.Latest()

	type response struct {
		vote finalizationVote

		cacheValid bool
		cacheBlock *Block
	}

	responseChan := make(chan response)

	for _, p := range peers {
		conn := p.Conn()
		cacheBlock := l.queryBlockCache[p.ID().Checksum()]

		f := func() {
			var response response
			defer func() {
				responseChan <- response
			}()

			req := &QueryRequest{BlockIndex: current.Index + 1}

			if cacheBlock != nil {
				req.CacheBlockId = make([]byte, SizeBlockID)
				copy(req.CacheBlockId, cacheBlock.ID[:])

				response.cacheBlock = cacheBlock
			}

			f := func() {
				client := NewWaveletClient(conn)

				ctx, cancel := context.WithTimeout(context.Background(), conf.GetQueryTimeout())
				defer cancel()

				p := &peer.Peer{}

				res, err := client.Query(ctx, req, grpc.Peer(p))
				if err != nil {
					logger := log.Node()
					logger.Error().
						Err(err).
						Msg("error while querying peer")

					return
				}

				l.metrics.queried.Mark(1)

				info := noise.InfoFromPeer(p)
				if info == nil {
					return
				}

				voter, ok := info.Get(skademlia.KeyID).(*skademlia.ID)
				if !ok {
					return
				}

				response.vote.voter = voter

				if res.CacheValid {
					response.cacheValid = true
					return
				}

				block, err := UnmarshalBlock(bytes.NewReader(res.GetBlock()))
				if err != nil {
					return
				}

				if block.ID == ZeroBlockID {
					return
				}

				response.vote.block = &block
			}
			l.metrics.queryLatency.Time(f)
		}

		l.queueWorkerPool.Queue(f)
	}

	votes := make([]*finalizationVote, 0, len(peers))
	voters := make(map[AccountID]struct{}, len(peers))

	for i := 0; i < cap(votes); i++ {
		response := <-responseChan

		if response.vote.voter == nil {
			continue
		}

		if _, recorded := voters[response.vote.voter.PublicKey()]; recorded {
			continue // To make sure the sampling process is fair, only allow one vote per peer.
		}

		voters[response.vote.voter.PublicKey()] = struct{}{}

		if response.cacheValid {
			response.vote.block = response.cacheBlock
		} else if response.vote.block != nil {
			l.queryBlockCache[response.vote.voter.Checksum()] = response.vote.block
		}

		votes = append(votes, &response.vote)
	}

	for _, vote := range votes {
		// Filter nil blocks
		if vote.block == nil {
			continue
		}

		// Filter blocks with unexpected block height
		if vote.block.Index != current.Index+1 {
			vote.block = nil
			continue
		}

		// Filter blocks with at least 1 missing tx
		if l.transactions.BatchMarkMissing(vote.block.Transactions...) {
			vote.block = nil
			continue
		}

		results, err := l.collapseTransactions(vote.block, false)
		if err != nil {
			logger := log.Node()
			logger.Error().
				Err(err).
				Msg("error collapsing transactions during query")
			continue
		}

		// Filter blocks that results in unexpected merkle root after collapse
		if results.snapshot.Checksum() != vote.block.Merkle {
			vote.block = nil
			continue
		}
	}

	TickForFinalization(l.accounts, l.finalizer, votes)
}

// SyncToLatestBlock continuously checks if the node is out of sync from its peers.
// If the majority of its peers responded that it is out of sync (decided using snowball),
// the node will attempt to sync its state to the latest block by downloading the AVL tree
// diff from its peers and applying the diff to its local AVL tree.
func (l *Ledger) SyncToLatestBlock() { // nolint:gocyclo,gocognit
	voteWG := new(sync.WaitGroup)

	snowballK := conf.GetSnowballK()
	syncVotes := make(chan *syncVote, snowballK)

	logger := log.Sync("sync")

	go CollectVotesForSync(l.accounts, l.syncer, syncVotes, voteWG, snowballK)

	syncTimeoutMultiplier := 0
	for {
		for {
			time.Sleep(5 * time.Millisecond)

			peers, err := SelectPeers(l.client.ClosestPeers(), snowballK)
			if err != nil {
				<-time.After(1 * time.Second)

				continue
			}

			current := l.blocks.Latest()

			var wg sync.WaitGroup
			wg.Add(len(peers))

			for _, p := range peers {
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

					syncVotes <- &syncVote{voter: voter, outOfSync: res.OutOfSync}

					wg.Done()
				}(p.Conn())
			}

			wg.Wait()

			if l.syncer.Decided() {
				break
			}
		}

		// Reset syncing Snowball sampler. Check if it is a false alarm such that we don't have to sync.

		current := l.blocks.Latest()
		preferred := l.syncer.Preferred()

		oos := *preferred.Value().(*bool)
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
			snowballK = conf.GetSnowballK()

			syncVotes = make(chan *syncVote, snowballK)
			go CollectVotesForSync(l.accounts, l.syncer, syncVotes, voteWG, snowballK)

			l.sync = make(chan struct{})
			l.PerformConsensus()
		}

		shutdown() // Shutdown all consensus-related workers.

		logger.Info().
			Uint64("current_block_index", current.Index).
			Msg("Noticed that we are out of sync; downloading latest state Snapshot from our peer(s).")

	SYNC:
		peers, err := SelectPeers(l.client.ClosestPeers(), conf.GetSnowballK())
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

		responses := make([]response, 0, len(peers))

		for _, p := range peers {
			stream, err := NewWaveletClient(p.Conn()).Sync(context.Background())
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

		var chunksBufferLock sync.Mutex
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

						if blake2b.Sum256(chunk) != src.checksum {
							continue
						}

						// We found the chunk! Store the chunks contents.
						chunksBufferLock.Lock()
						_, err = chunksBuffer.WriteAt(chunk, int64(src.idx)*sys.SyncChunkSize)
						chunksBufferLock.Unlock()

						if err != nil {
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

		_, err = l.blocks.Save(latest)
		if err != nil {
			logger.Error().
				Err(err).
				Msg("Failed to save finalized block to our database")

			cleanup()
			goto SYNC
		}

		if err := l.accounts.Commit(snapshot); err != nil {
			cleanup()
			logger := log.Node()
			logger.Fatal().Err(err).Msg("failed to commit collapsed state to our database")
		}

		l.resetTransactionsSyncIndex()

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
	idBuf := bytes.NewBuffer(make([]byte, SizeTransactionID*len(block.Transactions)))
	for _, id := range block.Transactions {
		idBuf.Write(id[:])
	}
	cacheKey := blake2b.Sum256(idBuf.Bytes())

	_collapseState, _ := l.cacheCollapse.LoadOrPut(cacheKey, &CollapseState{})
	collapseState := _collapseState.(*CollapseState)

	collapseState.once.Do(func() {
		txs := make([]*Transaction, 0, len(block.Transactions))

		for _, id := range block.Transactions {
			tx := l.transactions.Find(id)
			if tx == nil {
				collapseState.err = errors.Wrapf(ErrMissingTx, "%x", id)
				break
			}

			txs = append(txs, tx)
		}

		if collapseState.err != nil {
			return
		}

		collapseState.results, collapseState.err = collapseTransactions(txs, block, l.accounts)
	})

	if logging && collapseState.results != nil {
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

func (l *Ledger) resetTransactionsSyncIndex() {
	l.transactionsSyncIndexLock.Lock()
	defer l.transactionsSyncIndexLock.Unlock()

	l.transactionsSyncIndex.Reset()

	l.transactions.Iterate(func(tx *Transaction) bool {
		l.transactionsSyncIndex.Insert(tx.ID[:])
		return true
	})
}
