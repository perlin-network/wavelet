package main

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher/aead"
	"github.com/perlin-network/noise/handshake/ecdh"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	waveletnode "github.com/perlin-network/wavelet/node"
)

func spawnNode(port uint16) *noise.Node {
	params := noise.DefaultParams()
	params.Keys = skademlia.RandomKeys()
	params.Port = port

	node, err := noise.NewNode(params)
	if err != nil {
		panic(err)
	}

	p := protocol.New()
	p.Register(ecdh.New())
	p.Register(aead.New())
	p.Register(skademlia.New())
	p.Register(waveletnode.New())
	p.Enforce(node)

	go node.Listen()

	return node
}
