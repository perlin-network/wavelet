package node

import (
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/conflict"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"time"
)

var (
	ErrNoDiffFound = errors.New("sync: could not find a suitable diff to apply to the ledger")
)

type syncer struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	roots    map[common.TransactionID]*wavelet.Transaction
	accounts map[common.TransactionID]map[protocol.ID]struct{}
	resolver conflict.Resolver
}

func newSyncer(node *noise.Node) *syncer {
	return &syncer{
		node:     node,
		ledger:   Ledger(node),
		roots:    make(map[common.TransactionID]*wavelet.Transaction),
		accounts: make(map[common.TransactionID]map[protocol.ID]struct{}),
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

		var peerIDs []protocol.ID

		for peerID := range s.accounts[rootID] {
			peerIDs = append(peerIDs, peerID)
		}

		// Reset all state used for coming to consensus about the latest view-graph root.
		s.roots = make(map[common.TransactionID]*wavelet.Transaction)
		s.accounts = make(map[common.TransactionID]map[protocol.ID]struct{})

		// TODO(kenta): stop any broadcasting/querying

		fmt.Println(root)

		err := s.queryAndApplyDiff(peerIDs, root)
		if err != nil {
			fmt.Println(err)
		}

		// TODO(kenta): have the ledger start serving broadcasts/queries again
	}
}

func (s *syncer) addRootIfNotExists(account protocol.ID, root *wavelet.Transaction) {
	if _, exists := s.roots[root.ID]; !exists {
		s.roots[root.ID] = root
	}

	if _, instantiated := s.accounts[root.ID]; !instantiated {
		s.accounts[root.ID] = make(map[protocol.ID]struct{})
	}

	s.accounts[root.ID][account] = struct{}{}
}

func (s *syncer) getRootByID(id common.TransactionID) *wavelet.Transaction {
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

		if _, err := s.ledger.Reset(root, snapshot); err != nil {
			return err
		}
	}

	return ErrNoDiffFound
}
