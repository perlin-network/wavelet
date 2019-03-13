package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"golang.org/x/crypto/blake2b"
	"math/rand"
	"sync"
	"time"
)

type protocolID = [sizeProtocolID]byte

const sizeProtocolID = 90

type syncer struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	mu       sync.RWMutex
	accounts map[common.TransactionID]map[protocolID]struct{}

	resolver *wavelet.Snowball
}

func newSyncer(node *noise.Node) *syncer {
	return &syncer{
		node:     node,
		ledger:   Ledger(node),
		accounts: make(map[common.TransactionID]map[protocolID]struct{}),
		resolver: wavelet.NewSnowball().
			WithK(sys.SnowballSyncK).
			WithAlpha(sys.SnowballSyncAlpha).
			WithBeta(sys.SnowballSyncBeta),
	}
}

func (s *syncer) init() {
	go s.stateLoop()
	go s.transactionLoop()
}

func (s *syncer) stateLoop() {
	var root *wavelet.Transaction

	for {
		for {
			err := s.queryForLatestView()

			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if s.resolver.Decided() {
				root = s.resolver.Preferred()

				s.resolver.Reset()

				// The view ID we came to consensus to being the latest within the network
				// is less than or equal to ours. Go back to square one.
				if s.ledger.Root().ID == root.ID || s.ledger.ViewID() >= root.ViewID {
					time.Sleep(1 * time.Second)
					continue
				}

				break
			}
		}

		logger := log.Sync("new")
		logger.Info().
			Hex("peer_proposed_root_id", root.ID[:]).
			Uint64("peer_proposed_view_id", root.ViewID).
			Uint64("our_view_id", s.ledger.ViewID()).
			Msg("It looks like the majority of our peers has a larger view ID than us. Instantiating sync...")

		var peerIDs []protocol.ID

		s.mu.Lock()

		for peerID := range s.accounts[root.ID] {
			id, err := skademlia.ID{}.Read(payload.NewReader(peerID[:]))
			if err != nil {
				continue
			}

			peerIDs = append(peerIDs, id.(skademlia.ID))
		}

		// Reset all state used for coming to consensus about the latest view-graph root.
		s.accounts = make(map[common.TransactionID]map[protocolID]struct{})

		s.mu.Unlock()

		Broadcaster(s.node).Pause()

		if err := s.queryAndApplyDiff(peerIDs, root); err != nil {
			logger = log.Sync("error")
			logger.Error().
				Err(err).
				Msg("Failed to find and apply ledger state differences from our peers.")
		} else {
			logger = log.Sync("success")
			logger.Info().
				Hex("new_root_id", root.ID[:]).
				Uint64("new_view_id", s.ledger.ViewID()).
				Msg("Successfully synchronized with our peers.")
		}

		Broadcaster(s.node).Resume()
	}
}

func (s *syncer) transactionLoop() {
	opcodeSyncTransactionResponse, err := noise.OpcodeFromMessage((*SyncTransactionResponse)(nil))
	if err != nil {
		panic(errors.Wrap(err, "sync: response opcode not registered"))
	}

	logger := log.Sync("tx_sync")

	for {
		time.Sleep(100 * time.Millisecond)

		missingIDs, available := s.ledger.QueryMissingTransactions()
		if !available {
			continue
		}

		peerIDs, err := selectPeers(s.node, sys.SnowballSyncK)
		if err != nil || len(peerIDs) != sys.SnowballSyncK {
			continue
		}

		logger.Debug().Int("num_tx", len(missingIDs)).Msg("Syncing for missing transactions.")

		responses, err := broadcast(s.node, peerIDs, SyncTransactionRequest{ids: missingIDs}, opcodeSyncTransactionResponse)
		if err != nil {
			continue
		}

		for _, res := range responses {
			if res != nil {
				for _, tx := range res.(SyncTransactionResponse).transactions {
					if err := s.ledger.ReceiveTransaction(tx); errors.Cause(err) == wavelet.VoteRejected {
						logger.Debug().Err(err).Msg("Got an error.")
					}
				}
			}
		}
	}
}

func (s *syncer) recordRootFromAccount(accountID protocol.ID, rootID common.TransactionID) {
	s.mu.Lock()

	if _, instantiated := s.accounts[rootID]; !instantiated {
		s.accounts[rootID] = make(map[protocolID]struct{})
	}

	var id protocolID
	copy(id[:], accountID.Write())

	s.accounts[rootID][id] = struct{}{}

	s.mu.Unlock()
}

func (s *syncer) queryForLatestView() error {
	opcodeSyncViewResponse, err := noise.OpcodeFromMessage((*SyncViewResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "sync: response opcode not registered")
	}

	peerIDs, err := selectPeers(s.node, sys.SnowballSyncK)
	if err != nil {
		return errors.Wrap(err, "sync: cannot query for peer view IDs")
	}

	responses, err := broadcast(s.node, peerIDs, SyncViewRequest{root: s.ledger.Root()}, opcodeSyncViewResponse)
	if err != nil {
		return err
	}

	var accountIDs []common.AccountID

	for _, peerID := range peerIDs {
		var accountID common.AccountID
		copy(accountID[:], peerID.PublicKey())

		accountIDs = append(accountIDs, accountID)
	}

	votes := make(map[common.AccountID]common.TransactionID)
	transactions := make(map[common.TransactionID]*wavelet.Transaction)

	for i, res := range responses {
		if res != nil {
			if root := res.(SyncViewResponse).root; root != nil {
				s.recordRootFromAccount(peerIDs[i], root.ID)

				transactions[root.ID] = root
				votes[accountIDs[i]] = root.ID
			}
		}
	}

	weights := wavelet.ComputeStakeDistribution(s.ledger.Accounts, accountIDs, sys.SnowballSyncK)

	counts := make(map[common.TransactionID]float64)

	for account, preferredID := range votes {
		counts[preferredID] += weights[account]
	}

	s.resolver.Tick(counts, transactions)

	return nil
}

func (s *syncer) queryAndApplyDiff(peerIDs []protocol.ID, root *wavelet.Transaction) error {
	logger := log.Sync("query_and_apply_diff")

	type peerInfo struct {
		id     protocol.ID
		hashes [][blake2b.Size256]byte
	}

	opcodeSyncDiffMetadataResponse, err := noise.OpcodeFromMessage((*SyncDiffMetadataResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "sync: diff metadata response opcode not registered")
	}

	opcodeSyncDiffChunkResponse, err := noise.OpcodeFromMessage((*SyncDiffChunkResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "sync: diff chunk response opcode not registered")
	}

	responses, err := broadcast(s.node, peerIDs, SyncDiffMetadataRequest{viewID: s.ledger.Root().ViewID}, opcodeSyncDiffMetadataResponse)
	if err != nil {
		return errors.Wrap(err, "sync: failed to brodacast request for sync diffing metadata")
	}

	viewChunkHashes := make(map[uint64][]peerInfo)

	for i, res := range responses {
		if res != nil {
			res := res.(SyncDiffMetadataResponse)
			viewChunkHashes[res.latestViewID] = append(viewChunkHashes[res.latestViewID], peerInfo{
				id:     peerIDs[i],
				hashes: res.chunkHashes,
			})
		}
	}

	// Select a set of chunk hashes which more than 2/3rds of all
	// queried peers believes provides a sufficient diff from our
	// current view ID to our peers latest view ID.
	var selected []peerInfo

	for _, peers := range viewChunkHashes {
		if len(peers) >= len(peerIDs)*2/3 {
			selected = peers
			break
		}
	}

	if len(selected) == 0 {
		return errors.New("inconsistent view ids")
	}

	viewChunkHashes = nil

	type chunkSource struct {
		hash  [blake2b.Size256]byte
		peers []protocol.ID
	}

	var chunkSources []chunkSource

	for i := 0; ; i++ {
		hashCount := make(map[[blake2b.Size256]byte][]protocol.ID)
		hashInRange := false

		for _, peer := range selected {
			if i >= len(peer.hashes) {
				continue
			}

			hashCount[peer.hashes[i]] = append(hashCount[peer.hashes[i]], peer.id)
			hashInRange = true
		}

		if !hashInRange {
			break
		}

		consistent := false

		for hash, peers := range hashCount {
			if len(peers) >= len(selected)*2/3 && len(peers) > 0 {
				chunkSources = append(chunkSources, chunkSource{hash: hash, peers: peers})

				consistent = true
				break
			}
		}

		if !consistent {
			return errors.New("inconsistent chunk hashes")
		}
	}

	collectedChunks := make([][]byte, len(chunkSources))

	var numCollectedChunks atomic.Uint32

	var wg sync.WaitGroup
	wg.Add(len(chunkSources))

	for chunkID, src := range chunkSources {
		src := src

		// FIXME: Noise does not support concurrent request/response on a single peer.
		// go func() {
		func() {
			defer wg.Done()

			for i := 0; i < 5; i++ {
				peerID := src.peers[rand.Intn(len(src.peers))]

				peer := protocol.Peer(s.node, peerID)
				if peer == nil {
					continue
				}

				err := peer.SendMessage(SyncDiffChunkRequest{chunkHash: src.hash})
				if err != nil {
					continue
				}

				var res SyncDiffChunkResponse

				select {
				case msg := <-peer.Receive(opcodeSyncDiffChunkResponse):
					res = msg.(SyncDiffChunkResponse)
				case <-time.After(sys.QueryTimeout):
					continue
				}

				if !res.found {
					logger.Info().
						Hex("peer_id", peerID.PublicKey()).
						Msg("Chunk not found on remote peer.")
					continue
				}

				if remoteHash := blake2b.Sum256(res.diff); remoteHash != src.hash {
					logger.Info().
						Hex("remote_checksum", remoteHash[:]).
						Hex("source_checksum", src.hash[:]).
						Msg("Chunk hash mismatch.")

					continue
				}

				collectedChunks[chunkID] = res.diff
				numCollectedChunks.Add(1)

				break
			}
		}()
	}

	wg.Wait()

	if int(numCollectedChunks.Load()) != len(chunkSources) {
		return errors.New("failed to fetch some chunks from our peers")
	}

	var diff []byte

	for _, chunk := range collectedChunks {
		diff = append(diff, chunk...)
	}

	snapshot, err := s.ledger.Accounts.SnapshotAccounts().ApplyDiff(diff)
	if err != nil {
		return err
	}

	// The diff did not get us the intended merkle root we wanted. Skip.
	if snapshot.Checksum() != root.AccountsMerkleRoot {
		return errors.New("merkle root mismatch")
	}

	if err := s.ledger.Reset(root, snapshot); err != nil {
		return err
	}

	logger.Info().
		Int("num_chunks", len(collectedChunks)).
		Msg("Successfully built a new state tree out of chunk(s) we have received from peers.")

	return nil
}
