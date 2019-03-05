package node

import (
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
	"sync"
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
	//go s.work()
}

func (s *syncer) work() {
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

				if root = s.getRootByID(rootID); s.ledger.Root().ID == rootID || root.ViewID+1 <= s.ledger.ViewID() {
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

		// TODO(heyang): Implement TryRLock at places where RLock is used.
		s.ledger.ConsensusLock().Lock()

		err := s.queryAndApplyDiff(peerIDs, root)

		s.ledger.ConsensusLock().Unlock()

		if err != nil {
			logger = log.Sync("error")
			logger.Error().
				Err(err).
				Msg("Failed to find and apply ledger state differences from our peers.")
			continue
		}

		logger = log.Sync("success")
		logger.Info().
			Hex("new_root_id", rootID[:]).
			Uint64("new_view_id", s.ledger.ViewID()).
			Msg("Successfully synchronized with our peers.")

		// TODO(kenta): have the ledger start serving broadcasts/queries again
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

	peerIDs, responses, err := broadcast(s.node, SyncViewRequest{root: s.ledger.Root()}, opcodeSyncViewResponse)
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
	opcodeSyncDiffResponse, err := noise.OpcodeFromMessage((*SyncDiffResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "sync: response opcode not registered")
	}

	req := &SyncDiffRequest{viewID: s.ledger.ViewID()}

	for _, peerID := range peerIDs {
		var account common.AccountID
		copy(account[:], peerID.PublicKey())

		peer := protocol.Peer(s.node, peerID)
		if peer == nil {
			continue
		}

		// Send query request.
		err := peer.SendMessage(req)
		if err != nil {
			continue
		}

		var res SyncDiffResponse

		select {
		case msg := <-peer.Receive(opcodeSyncDiffResponse):
			res = msg.(SyncDiffResponse)
		case <-time.After(sys.QueryTimeout):
			continue
		}

		// The peer that originally gave us the root is giving us a different root! Skip.
		if res.root.ID != root.ID {
			continue
		}

		snapshot, err := s.ledger.SnapshotAccounts().ApplyDiff(res.diff)
		if err != nil {
			continue
		}

		// The diff did not get us the intended merkle root we wanted. Skip.
		if snapshot.Checksum() != root.AccountsMerkleRoot {
			continue
		}

		// TODO(kenta): sync difficulty and view-graph height

		if _, err := s.ledger.Reset(root, snapshot); err != nil {
			return err
		}

		return nil
	}

	return ErrNoDiffFound
}
