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

func (s *syncer) work() {
	for {
		err := s.queryForViewID()

		if err != nil {
			panic(err)
			continue
		}

		time.Sleep(1 * time.Second)
	}
}

func (s *syncer) queryForViewID() error {
	opcodeSyncViewResponse, err := noise.OpcodeFromMessage((*SyncViewResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "sync: response opcode not registered")
	}

	accounts, responses, err := broadcast(s.node, SyncViewRequest{viewID: s.ledger.ViewID()}, opcodeSyncViewResponse)
	if err != nil {
		return err
	}

	votes := make(map[common.AccountID]uint64)
	for i, res := range responses {
		if res != nil {
			votes[accounts[i]] = res.(SyncViewResponse).root.ViewID
		}
	}

	weights := s.ledger.ComputeStakeDistribution(accounts)

	counts := make(map[interface{}]float64)

	for account, preferred := range votes {
		counts[preferred] += weights[account]
	}

	return nil
}
