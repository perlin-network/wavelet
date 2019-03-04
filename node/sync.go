package node

import (
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/conflict"
	"github.com/pkg/errors"
	"time"
)

type syncer struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	roots    map[common.TransactionID]*wavelet.Transaction
	resolver conflict.Resolver
}

func newSyncer(node *noise.Node) *syncer {
	return &syncer{node: node, ledger: Ledger(node), roots: make(map[common.TransactionID]*wavelet.Transaction), resolver: conflict.NewSnowball()}
}

func (s *syncer) init() {
	//go s.work()
}

func (s *syncer) work() {
	var rootID common.TransactionID
	var root *wavelet.Transaction

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

	// Reset all state used for coming to consensus about the latest view-graphr oot.
	s.roots = make(map[common.TransactionID]*wavelet.Transaction)

	// TODO(kenta): stop any broadcasting/querying and download account state tree diffs
	fmt.Println(root)
}

func (s *syncer) addRootIfNotExists(root *wavelet.Transaction) {
	if _, exists := s.roots[root.ID]; !exists {
		s.roots[root.ID] = root
	}
}

func (s *syncer) getRootByID(id common.TransactionID) *wavelet.Transaction {
	return s.roots[id]
}

func (s *syncer) queryForLatestView() error {
	opcodeSyncViewResponse, err := noise.OpcodeFromMessage((*SyncViewResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "sync: response opcode not registered")
	}

	accounts, responses, err := broadcast(s.node, SyncViewRequest{root: s.ledger.Root()}, opcodeSyncViewResponse)
	if err != nil {
		return err
	}

	votes := make(map[common.AccountID]common.TransactionID)
	for i, res := range responses {
		if res != nil {
			root := res.(SyncViewResponse).root
			s.addRootIfNotExists(root)

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
