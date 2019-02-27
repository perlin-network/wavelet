package net

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
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
		// Only continue to gossip critical transactions that are not out-of-sync
		// with our current view ID.

		if status.viewID == ledger.ViewID() {
			err := QueryTransaction(peer.Node(), status.tx)

			if err != nil {
				log.Warn().Err(err).Msgf("Failed to further gossip out transaction %x.", status.tx.ID)
			}

			status.count++

			log.Debug().Int("count", status.count).Hex("tx_id", status.tx.ID[:]).Msg("Gossiped out transaction.")

			if status.tx.IsCritical(ledger.Difficulty()) {
				ch <- status
			}
		}
	}
}
