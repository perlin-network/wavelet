package api

import (
	"time"

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
	Time      time.Time         `json:"time"`
}

var _ log.Loggable = (*NetworkJoined)(nil)

func (n *NetworkJoined) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("public_key", n.PublicKey[:])
	ev.Str("address", n.Address)
	ev.Str("time", time.Now().Format(time.RFC3339Nano))
	ev.Msg("Peer has joined.")
}

func (n *NetworkJoined) UnmarshalValue(v *fastjson.Value) error {
	if err := log.ValueBatch(v,
		"public_key", n.PublicKey,
		"address", &n.Address); err != nil {

		return err
	}

	log.ValueHex(v, n.PublicKey, "public_key")

	t, err := log.ValueTime(v, time.RFC3339Nano, "time")
	if err != nil {
		return err
	}
	n.Time = *t

	return nil
}

// event: left
type NetworkLeft struct {
	PublicKey wavelet.AccountID `json:"public_key"`
	Address   string            `json:"address"`
	Time      time.Time         `json:"time"`
}

var _ log.Loggable = (*NetworkLeft)(nil)

func (n *NetworkLeft) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("public_key", n.PublicKey[:])
	ev.Str("address", n.Address)
	ev.Str("time", time.Now().Format(time.RFC3339Nano))
	ev.Msg("Peer has left.")
}

func (n *NetworkLeft) UnmarshalValue(v *fastjson.Value) error {
	if err := log.ValueBatch(v,
		"public_key", n.PublicKey,
		"address", &n.Address); err != nil {

		return err
	}

	t, err := log.ValueTime(v, time.RFC3339Nano, "time")
	if err != nil {
		return err
	}
	n.Time = *t

	return nil
}

func (g *Gateway) registerEvents() {
	if g.Config != nil && g.Client != nil {
		g.Client.OnPeerJoin(func(conn *grpc.ClientConn, id *skademlia.ID) {
			log.EventTo(log.Network("joined").Info(), &NetworkJoined{
				PublicKey: id.PublicKey(),
				Address:   id.Address(),
				Time:      time.Now(),
			})
		})

		g.Client.OnPeerLeave(func(conn *grpc.ClientConn, id *skademlia.ID) {
			log.EventTo(log.Network("left").Info(), &NetworkLeft{
				PublicKey: id.PublicKey(),
				Address:   id.Address(),
				Time:      time.Now(),
			})
		})
	}
}
