// +build integration

package wavelet

import (
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher"
	"github.com/perlin-network/noise/handshake"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

func TestSelectPeers(t *testing.T) {
	nodes := make([]*skademlia.Client, 10)
	addrs := make([]string, 10)
	cleanup := make([]func(), 10)

	var err error
	for i := 0; i < 10; i++ {
		nodes[i], addrs[i], cleanup[i], err = newNode()
		if cleanup[i] != nil {
			defer cleanup[i]()
		}

		assert.NoError(t, err)
	}

	for i := 0; i < 10; i++ {
		_, err := nodes[i].Dial(addrs[(i+1)%10])
		assert.NoError(t, err)
	}

	for i := 0; i < 10; i++ {
		nodes[i].Bootstrap()
	}

	closest := nodes[0].ClosestPeers()

	selected, err := SelectPeers(closest, 5)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(selected))

	// Close 5 of the nodes, leaving node[0] with only 4 active peers
	for i := 5; i < 10; i++ {
		cleanup[i]()
	}

	// Wait for grpc.Client to detect the closed connections
	timeout := time.NewTimer(time.Millisecond * 100)
	ticker := time.NewTicker(time.Millisecond * 10)
	activeCount := len(closest)

	for activeCount > 4 {
		activeCount = 0
		select {
		case <-ticker.C:
			for _, peer := range closest {
				if peer.Conn().GetState() == connectivity.Ready {
					activeCount++
				}
			}

		case <-timeout.C:
			t.Fatal("test timed out")
		}
	}

	// SelectPeers should only return 4 peers
	selected, err = SelectPeers(closest, 5)
	assert.NoError(t, err)
	assert.Equal(t, 4, len(selected))

	// Calling ClosestPeers() should return 4 peers
	closest = nodes[0].ClosestPeers()
	selected, err = SelectPeers(closest, 5)
	assert.EqualError(t, err, "only connected to 4 peer(s), but require a minimum of 5 peer(s)")
	assert.Equal(t, 4, len(selected))
}

func newNode() (*skademlia.Client, string, func(), error) {
	keys, err := skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		return nil, "", nil, err
	}

	ln, err := net.Listen("tcp", ":0") // nolint:gosec
	if err != nil {
		return nil, "", nil, err
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))

	client := skademlia.NewClient(addr, keys, skademlia.WithC1(sys.SKademliaC1), skademlia.WithC2(sys.SKademliaC2))
	client.SetCredentials(noise.NewCredentials(addr, handshake.NewECDH(), cipher.NewAEAD(), client.Protocol()))

	kv, cleanup, err := store.NewTestKV("level", "db")
	if err != nil {
		return nil, "", nil, err
	}

	ledger, err := NewLedger(kv, client, WithoutGC())
	if err != nil {
		return nil, "", cleanup, err
	}

	server := client.Listen()
	RegisterWaveletServer(server, ledger.Protocol())

	go func() { // nolint:staticcheck
		if err := server.Serve(ln); err != nil && err != grpc.ErrServerStopped {
			fmt.Println(err)
		}
	}()

	return client, addr, func() {
		server.GracefulStop()
		cleanup()
	}, nil
}
