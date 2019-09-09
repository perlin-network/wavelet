package main

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/wctl"
)

func setEvents(c *wctl.Client) error {
	c.OnPeerJoin = onPeerJoin
	c.OnPeerLeave = onPeerLeave
	if _, err := c.PollNetwork(); err != nil {
		return err
	}

	return nil
}

func onPeerJoin(u wctl.PeerJoin) {
	logger := log.Network("joined")
	logger.Info().
		Hex("public_key", u.AccountID[:]).
		Str("address", u.Address).
		Msg("Peer has joined.")
}

func onPeerLeave(u wctl.PeerLeave) {
	logger := log.Network("left")
	logger.Info().
		Hex("public_key", u.AccountID[:]).
		Str("address", u.Address).
		Msg("Peer has joined.")
}
