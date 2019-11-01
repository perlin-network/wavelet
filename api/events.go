package api

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
	"google.golang.org/grpc"
)

// event: joined
type NetworkJoined struct {
	PublicKey wavelet.AccountID `json:"public_key"`
	Address   string            `json:"address"`
}

var _ log.Loggable = (*NetworkJoined)(nil)

func (n *NetworkJoined) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("public_key", n.PublicKey[:])
	ev.Str("address", n.Address)
	ev.Msg("Peer has joined.")
}

func (n *NetworkJoined) UnmarshalValue(v *fastjson.Value) error {
	n.Address = string(v.GetStringBytes("address"))
	return log.ValueHex(v, n.PublicKey, "public_key")
}

// event: left
type NetworkLeft struct {
	PublicKey wavelet.AccountID `json:"public_key"`
	Address   string            `json:"address"`
}

var _ log.Loggable = (*NetworkLeft)(nil)

func (n *NetworkLeft) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("public_key", n.PublicKey[:])
	ev.Str("address", n.Address)
	ev.Msg("Peer has left.")
}

func (n *NetworkLeft) UnmarshalValue(v *fastjson.Value) error {
	n.Address = string(v.GetStringBytes("address"))
	return log.ValueHex(v, n.PublicKey, "public_key")
}

func (g *Gateway) registerEvents() {
	if g.Config != nil && g.Client != nil {
		g.Client.OnPeerJoin(func(conn *grpc.ClientConn, id *skademlia.ID) {
			log.EventTo(log.Network("joined").Info(), &NetworkJoined{
				PublicKey: id.PublicKey(),
				Address:   id.Address(),
			})
		})

		g.Client.OnPeerLeave(func(conn *grpc.ClientConn, id *skademlia.ID) {
			log.EventTo(log.Network("left").Info(), &NetworkLeft{
				PublicKey: id.PublicKey(),
				Address:   id.Address(),
			})
		})
	}
}
