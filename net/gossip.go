package net

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
)

const keyGossipChannel = "wavelet.gossip.ch"

type gossipStatus struct {
	tx *wavelet.Transaction

	viewID uint64
	count  int
}

func gossipOutTransaction(node *noise.Node, tx *wavelet.Transaction) {
	ch := node.LoadOrStore(keyGossipChannel, make(chan gossipStatus, 1024)).(chan gossipStatus)

	ch <- gossipStatus{
		tx:     tx,
		viewID: Ledger(node).ViewID(),
		count:  0,
	}
}

func gossipLoop(ledger *wavelet.Ledger, peer *noise.Peer) {
	ch := peer.Node().LoadOrStore(keyGossipChannel, make(chan gossipStatus, 1024)).(chan gossipStatus)

	for status := range ch {
		err := QueryTransaction(peer.Node(), status.tx)

		if err != nil {
			log.Warn().Err(err).Msgf("Failed to further gossip out transaction %x.", status.tx.ID)
		}

		log.Debug().Int("count", status.count+1).Hex("tx_id", status.tx.ID[:]).Msg("Gossiped out transaction.")

		status.count++

		// Only continue to gossip if we are still in the same consensus view ID, and if
		// we gossiped less than or equal to SNOWBALL_BETA times.
		if status.count <= sys.SnowballBeta+1 && status.viewID == ledger.ViewID() {
			ch <- status
		}
	}
}
