package node

import (
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

	resolver conflict.Resolver
}

func newSyncer(node *noise.Node) *syncer {
	return &syncer{node: node, ledger: Ledger(node), resolver: conflict.NewSnowball()}
}

func (s *syncer) init() {
	//go s.work()
}

func (s *syncer) work() {
	var root wavelet.Transaction

	for {
		err := s.queryForLatestView()

		if err != nil {
			panic(err)
			continue
		}

		if s.resolver.Decided() {
			// The view ID we came to consensus to being the latest within the network
			// is less than or equal to ours. Go back to square one.
			if root = s.resolver.Preferred().(wavelet.Transaction); root.ViewID <= s.ledger.ViewID() {
				time.Sleep(1 * time.Second)

				continue
			}

			break
		}

		time.Sleep(1 * time.Millisecond)
	}

	s.resolver.Reset()

	// TODO(kenta): stop any broadcasting/querying and download account state tree diffs
}

func (s *syncer) queryForLatestView() error {
	opcodeSyncViewResponse, err := noise.OpcodeFromMessage((*SyncViewResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "sync: response opcode not registered")
	}

	accounts, responses, err := broadcast(s.node, SyncViewRequest{root: *s.ledger.Root()}, opcodeSyncViewResponse)
	if err != nil {
		return err
	}

	votes := make(map[common.AccountID]wavelet.Transaction)
	for i, res := range responses {
		if res != nil {
			votes[accounts[i]] = res.(SyncViewResponse).root
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
