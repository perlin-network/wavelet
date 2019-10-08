package api

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/log"
	"google.golang.org/grpc"
)

func (g *Gateway) registerEvents() {
	if g.Config != nil && g.Client != nil {
		g.Client.OnPeerJoin(func(conn *grpc.ClientConn, id *skademlia.ID) {
			publicKey := id.PublicKey()

			logger := log.Network("joined")
			logger.Info().
				Hex("public_key", publicKey[:]).
				Str("address", id.Address()).
				Msg("Peer has joined.")
		})

		g.Client.OnPeerLeave(func(conn *grpc.ClientConn, id *skademlia.ID) {
			publicKey := id.PublicKey()

			logger := log.Network("left")
			logger.Info().
				Hex("public_key", publicKey[:]).
				Str("address", id.Address()).
				Msg("Peer has left.")
		})
	}
}
