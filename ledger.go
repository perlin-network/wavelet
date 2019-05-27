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
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	queue2 "github.com/phf/go-queue/queue"
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

func NewLedger(kv store.KV, client *skademlia.Client) *Ledger {
	metrics := NewMetrics(context.TODO())

	accounts := NewAccounts(kv)
	go accounts.GC(context.Background())

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
	go ledger.PerformConsensus()

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
	go l.FinalizeRounds()
}

func (l *Ledger) Snapshot() *avl.Tree {
	return l.accounts.Snapshot()
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

		peers := l.client.ClosestPeers()

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
		l.metrics.receivedTX.Mark(count)

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

		if len(l.client.ClosestPeers()) < sys.SnowballK {
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

						if round.End.Depth <= round.Start.Depth {
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

						if err := l.AddTransaction(round.Start); err != nil {
							return
						}

						if err := l.AddTransaction(round.End); err != nil {
							return
						}

						if !round.End.IsCritical(currentDifficulty) {
							return
						}

						results, err := l.CollapseTransactions(round.Index, round.Start, round.End, false)
						if err != nil {
							fmt.Println(err)
							return
						}

						if uint64(results.appliedCount) != round.Applied {
							fmt.Printf("applied %d but expected %d\n", results.appliedCount, round.Applied)
							return
						}

						if results.snapshot.Checksum() != round.Merkle {
							fmt.Printf("got merkle %x but expected %x\n", results.snapshot.Checksum(), round.Merkle)
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

			peers, err := SelectPeers(l.client.ClosestPeers(), sys.SnowballK)
			if err != nil {
				close(workerChan)
				workerWG.Wait()
				workerWG.Add(1)
				close(voteChan)
				workerWG.Wait() // Wait for vote processor worker to close.

				continue FINALIZE_ROUNDS
			}

			for _, peer := range peers {
				workerChan <- peer
			}
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

		l.graph.UpdateRootDepth(finalized.End.Depth)

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

					if round.End.Depth <= round.Start.Depth {
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

			l.syncTimer.Reset((1500 / (1 + 2*time.Duration(l.syncer.Progress()))) * time.Millisecond)

			select {
			case <-l.syncTimer.C:
			}
		}

		// Reset syncing Snowball sampler. Check if it is a false alarm such that we don't have to sync.

		current := l.rounds.Latest()
		proposed := l.syncer.Preferred()

		if proposed.Index < sys.SyncIfRoundsDifferBy+current.Index {
			l.syncer.Reset()
			continue
		}

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
			go CollectVotes(l.accounts, l.syncer, l.syncVotes, voteWG)

			l.sync = make(chan struct{})
			go l.PerformConsensus()
		}

		shutdown() // Shutdown all consensus-related workers.

		logger := log.Sync("syncing")
		logger.Info().
			Uint64("current_round", current.Index).
			Uint64("proposed_round", proposed.Index).
			Msg("Noticed that we are out of sync; downloading latest state tree from our peer(s).")

	SYNC:

		conns, err := SelectPeers(l.client.ClosestPeers(), sys.SnowballK)
		if err != nil {
			logger.Warn().Msg("It looks like there are no peers for us to sync with. Retrying...")

			select {
			case <-time.After(1 * time.Second):
			}

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
			panic(errors.Wrap(err, "failed to commit collapsed state to our database"))
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
			Msg("Successfully built a new state tree out of chunk(s) we have received from peers.")

		restart()
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
func (l *Ledger) CollapseTransactions(round uint64, root Transaction, end Transaction, logging bool) (CollapseResults, error) {
	if results, exists := l.cacheCollapse.load(end.ID); exists {
		return results.(CollapseResults), nil
	}

	var results CollapseResults

	results.snapshot = l.accounts.Snapshot()
	results.snapshot.SetViewID(round)

	visited := map[TransactionID]struct{}{root.ID: {}}

	queue := queue2.New()
	//defer ReleaseQueue(queue)

	queue.PushBack(&end)

	order := queue2.New()
	//defer ReleaseQueue(order)

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		if popped.Depth <= root.Depth {
			continue
		}

		order.PushBack(popped)

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

			queue.PushBack(parent)
		}
	}

	// Apply transactions in reverse order from the end of the round
	// all the way down to the beginning of the round.

	for order.Len() > 0 {
		popped := order.PopBack().(*Transaction)

		if err := l.RewardValidators(results.snapshot, root, popped, logging); err != nil {
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

			fmt.Println(err)

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

	startDepth, endDepth := root.Depth+1, end.Depth

	for _, tx := range l.graph.GetTransactionsByDepth(&startDepth, &endDepth) {
		results.ignoredCount += tx.LogicalUnits()
	}

	results.ignoredCount -= results.appliedCount + results.rejectedCount

	l.cacheCollapse.put(end.ID, results)
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
