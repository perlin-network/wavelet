package node

import (
	"encoding/hex"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/conflict"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

type protocolID = [sizeProtocolID]byte

const sizeProtocolID = 90

var (
	ErrNoDiffFound = errors.New("sync: could not find a suitable diff to apply to the ledger")
)

type syncer struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	mu       sync.RWMutex
	roots    map[common.TransactionID]*wavelet.Transaction
	accounts map[common.TransactionID]map[protocolID]struct{}

	resolver conflict.Resolver
}

func newSyncer(node *noise.Node) *syncer {
	return &syncer{
		node:     node,
		ledger:   Ledger(node),
		roots:    make(map[common.TransactionID]*wavelet.Transaction),
		accounts: make(map[common.TransactionID]map[protocolID]struct{}),
		resolver: conflict.NewSnowball(),
	}
}

func (s *syncer) init() {
	go s.loop()
}

func (s *syncer) loop() {
	var rootID common.TransactionID
	var root *wavelet.Transaction

	for {
		for {
			err := s.queryForLatestView()

			if err != nil {
				continue
			}

			if s.resolver.Decided() {
				// The view ID we came to consensus to being the latest within the network
				// is less than or equal to ours. Go back to square one.
				rootID = s.resolver.Preferred().(common.TransactionID)

				s.resolver.Reset()

				if root = s.getRootByID(rootID); s.ledger.Root().ID == rootID || s.ledger.ViewID() >= root.ViewID+1 {
					time.Sleep(1 * time.Second)
					continue
				}

				break
			}

			time.Sleep(1 * time.Millisecond)
		}

		logger := log.Sync("new")
		logger.Info().
			Hex("peer_proposed_root_id", rootID[:]).
			Uint64("peer_proposed_view_id", root.ViewID).
			Uint64("our_view_id", s.ledger.ViewID()).
			Msg("It looks like the majority of our peers has a larger view ID than us. Instantiating sync...")

		var peerIDs []protocol.ID

		for peerID := range s.accounts[rootID] {
			id, err := skademlia.ID{}.Read(payload.NewReader(peerID[:]))
			if err != nil {
				continue
			}

			peerIDs = append(peerIDs, id.(skademlia.ID))
		}

		// Reset all state used for coming to consensus about the latest view-graph root.
		s.mu.Lock()
		s.roots = make(map[common.TransactionID]*wavelet.Transaction)
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
				Hex("new_root_id", rootID[:]).
				Uint64("new_view_id", s.ledger.ViewID()).
				Msg("Successfully synchronized with our peers.")
		}

		Broadcaster(s.node).Resume()
	}
}

func (s *syncer) addRootIfNotExists(account protocol.ID, root *wavelet.Transaction) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.roots[root.ID]; !exists {
		s.roots[root.ID] = root
	}

	if _, instantiated := s.accounts[root.ID]; !instantiated {
		s.accounts[root.ID] = make(map[protocolID]struct{})
	}

	var id protocolID
	copy(id[:], account.Write())

	s.accounts[root.ID][id] = struct{}{}
}

func (s *syncer) getRootByID(id common.TransactionID) *wavelet.Transaction {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.roots[id]
}

func (s *syncer) queryForLatestView() error {
	opcodeSyncViewResponse, err := noise.OpcodeFromMessage((*SyncViewResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "sync: response opcode not registered")
	}

	peerIDs, err := selectPeers(s.node, sys.SnowballK)
	if err != nil {
		return errors.Wrap(err, "sync: cannot query for peer view IDs")
	}

	responses, err := broadcast(s.node, peerIDs, SyncViewRequest{root: s.ledger.Root()}, opcodeSyncViewResponse)
	if err != nil {
		return err
	}

	var accounts []common.AccountID
	for _, peerID := range peerIDs {
		var account common.AccountID
		copy(account[:], peerID.PublicKey())

		accounts = append(accounts, account)
	}

	votes := make(map[common.AccountID]common.TransactionID)
	for i, res := range responses {
		if res != nil {
			root := res.(SyncViewResponse).root
			s.addRootIfNotExists(peerIDs[i], root)

			votes[accounts[i]] = root.ID
		}
	}

	weights := s.ledger.ComputeStakeDistribution(accounts)

	counts := make(map[interface{}]float64)

	for account, preferred := range votes {
		counts[preferred] += weights[account]
	}

	s.resolver.Tick(counts)

	return nil
}

func (s *syncer) queryAndApplyDiff(peerIDs []protocol.ID, root *wavelet.Transaction) error {
	logger := log.Sync("query_and_apply_diff")

	type peerInfo struct {
		id     protocol.ID
		hashes [][]byte
	}
	opcodeSyncDiffMetadataResponse, err := noise.OpcodeFromMessage((*SyncDiffMetadataResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "sync: response opcode not registered")
	}

	opcodeSyncDiffChunkResponse, err := noise.OpcodeFromMessage((*SyncDiffChunkResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "sync: response opcode not registered")
	}

	req := &SyncDiffMetadataRequest{viewID: s.ledger.Root().ViewID}
	viewIDHashes := make(map[uint64][]peerInfo)
	var selectedPeerSet []peerInfo

	wg := sync.WaitGroup{}
	viHashMutex := sync.Mutex{}

	for _, peerID := range peerIDs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			peer := protocol.Peer(s.node, peerID)
			if peer == nil {
				return
			}

			// Send query request.
			err := peer.SendMessage(req)
			if err != nil {
				return
			}

			var res SyncDiffMetadataResponse

			select {
			case msg := <-peer.Receive(opcodeSyncDiffMetadataResponse):
				res = msg.(SyncDiffMetadataResponse)
			case <-time.After(sys.QueryTimeout):
				return
			}

			viHashMutex.Lock()
			viewIDHashes[res.latestViewID] = append(viewIDHashes[res.latestViewID], peerInfo{
				id:     peerID,
				hashes: res.chunkHashes,
			})
			viHashMutex.Unlock()
		}()
	}
	wg.Wait()

	for _, peers := range viewIDHashes {
		if len(peers) >= len(peerIDs)*2/3 {
			selectedPeerSet = peers
			break
		}
	}

	if len(selectedPeerSet) == 0 {
		return errors.New("inconsistent view ids")
	}

	viewIDHashes = nil

	type chunkSource struct {
		hash  [blake2bHashSize]byte
		peers []protocol.ID
	}

	chunkSources := make([]chunkSource, 0)

	for i := 0; ; i++ {
		hashCount := make(map[[blake2bHashSize]byte][]protocol.ID)
		hashInRange := false
		for _, peer := range selectedPeerSet {
			if i >= len(peer.hashes) {
				continue
			}
			hashInRange = true
			var hash [blake2bHashSize]byte
			copy(hash[:], peer.hashes[i])
			hashCount[hash] = append(hashCount[hash], peer.id)
		}
		if !hashInRange {
			break
		}

		consistent := false

		for hash, ps := range hashCount {
			if len(ps) >= len(selectedPeerSet)*2/3 && len(ps) > 0 {
				chunkSources = append(chunkSources, chunkSource{hash, ps})
				consistent = true
				break
			}
		}

		if !consistent {
			return errors.New("inconsistent chunk hashes")
		}
	}

	collectedChunks := make([][]byte, len(chunkSources))
	successCount := uint32(0)

	for chunkID, cs := range chunkSources {
		cs := cs
		wg.Add(1)

		// FIXME: Noise does not support concurrent request/response on a single peer.
		// go func() {
		func() {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				peerID := cs.peers[rand.Intn(len(cs.peers))]
				peer := protocol.Peer(s.node, peerID)
				if peer == nil {
					continue
				}

				// Send query request.
				err := peer.SendMessage(&SyncDiffChunkRequest{hash: cs.hash[:]})
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
					logger.Info().Msg("Chunk not found on remote peer")
					continue
				}

				remoteHash := blake2b.Sum256(res.diff)
				if remoteHash != cs.hash {
					logger.Info().Msgf("Chunk hash mismatch: %s %s", hex.EncodeToString(remoteHash[:]), hex.EncodeToString(cs.hash[:]))
					continue
				}

				collectedChunks[chunkID] = res.diff
				atomic.AddUint32(&successCount, 1)
				break
			}
		}()
	}

	wg.Wait()
	if int(successCount) != len(chunkSources) {
		return errors.New("failed to fetch some chunks from our peers")
	}

	var diff []byte
	for _, chunk := range collectedChunks {
		diff = append(diff, chunk...)
	}

	snapshot, err := s.ledger.SnapshotAccounts().ApplyDiff(diff)
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

	logger.Info().Msgf("Successfully built a new state tree from %d chunk(s).", len(collectedChunks))

	return nil
}
