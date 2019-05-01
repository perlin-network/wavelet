package node

import (
	"github.com/fortytw2/leaktest"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher"
	"github.com/perlin-network/noise/handshake"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/noise/xnoise"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"net"
	"strconv"
	"testing"
	"time"
)

func protocol(n *noise.Node, keys *skademlia.Keypair) (*Protocol, *skademlia.Protocol, noise.Protocol) {
	ecdh := handshake.NewECDH()
	ecdh.RegisterOpcodes(n)

	aead := cipher.NewAEAD()
	aead.RegisterOpcodes(n)

	overlay := skademlia.New(net.JoinHostPort("127.0.0.1", strconv.Itoa(n.Addr().(*net.TCPAddr).Port)), keys, xnoise.DialTCP)
	overlay.RegisterOpcodes(n)
	overlay.WithC1(sys.SKademliaC1)
	overlay.WithC2(sys.SKademliaC2)

	w := New(overlay, keys, store.NewInmem(), nil)
	w.RegisterOpcodes(n)
	w.Init(n)

	protocol := noise.NewProtocol(xnoise.LogErrors, ecdh.Protocol(), aead.Protocol(), overlay.Protocol(), w.Protocol())

	return w, overlay, protocol
}

func TestProtocolLeak(t *testing.T) {
	defer leaktest.Check(t)()

	n, err := xnoise.ListenTCP(0)
	if !assert.NoError(t, err) {
		return
	}

	k, err := skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	if !assert.NoError(t, err) {
		return
	}

	w, _, p := protocol(n, k)
	n.FollowProtocol(p)

	time.Sleep(100 * time.Millisecond)

	w.Stop()
	n.Shutdown()
}
