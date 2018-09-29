package node

import (
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/peer"
)

type sybil interface {
	weigh(peers []peer.ID, responses []bool, tx *wire.Transaction) (positives float32)
}
