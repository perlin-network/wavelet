package api

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/log"
	"google.golang.org/grpc"
)

func (g *Gateway) registerEvents() {
	if g.Config != nil && g.Client != nil {
		g.Client.OnPeerJoin(func(conn *grpc.ClientConn, id *skademlia.ID) {
			log.EventTo(log.Network("left").Info(), log.NetworkJoined{
				PublicKey: id.PublicKey(),
				Address:   id.Address(),
			})
		})

		g.Client.OnPeerLeave(func(conn *grpc.ClientConn, id *skademlia.ID) {
			log.EventTo(log.Network("left").Info(), log.NetworkLeft{
				PublicKey: id.PublicKey(),
				Address:   id.Address(),
			})
		})
	}
}
