package node

import (
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/conflict"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

type syncer struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	viewResolver conflict.Resolver
}

func (s *syncer) work() {
	for {
		_, err := s.queryLatestViewID()

		if err != nil {
			fmt.Println(err)
			continue
		}
	}
}

func (s *syncer) selectPeers(amount int) ([]protocol.ID, error) {
	peerIDs := skademlia.FindClosestPeers(skademlia.Table(s.node), protocol.NodeID(s.node).Hash(), amount)

	if len(peerIDs) < sys.SnowballK {
		return nil, errors.Errorf("sync: only connected to %d peer(s) "+
			"but require a minimum of %d peer(s)", len(peerIDs), sys.SnowballK)
	}

	return peerIDs, nil
}

func (s *syncer) queryLatestViewID() (uint64, error) {
	//opcode, err := noise.OpcodeFromMessage((*SyncViewResponse)(nil))
	//if err != nil {
	//	return 0, errors.Wrap(err, "sync: sync response opcode not registered")
	//}
	//
	//peerIDs, err := s.selectPeers(sys.SnowballK)
	//if err != nil {
	//	return 0, err
	//}
	//
	//var mu sync.Mutex
	//var wg sync.WaitGroup
	//wg.Add(len(peerIDs))
	//
	//responses := make(map[common.TransactionID]bool)

	//recordResponse := func(rawID []byte, response bool) {
	//	mu.Lock()
	//	defer mu.Unlock()
	//
	//	var id common.TransactionID
	//	copy(id[:], rawID)
	//
	//	responses[id] = response
	//}

	return 0, nil
}
