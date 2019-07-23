package wavelet

import (
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher"
	"github.com/perlin-network/noise/handshake"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestSelectPeers(t *testing.T) {
	nodes := make([]*skademlia.Client, 10)
	addrs := make([]string, 10)
	cleanup := make([]func(), 10)
	for i := 0; i < 10; i++ {
		nodes[i], addrs[i], cleanup[i] = newNode(t)
		defer cleanup[i]()
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

func newNode(t *testing.T) (*skademlia.Client, string, func()) {
	keys, err := skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	assert.NoError(t, err)

	ln, err := net.Listen("tcp", ":0")
	assert.NoError(t, err)

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))

	client := skademlia.NewClient(addr, keys, skademlia.WithC1(sys.SKademliaC1), skademlia.WithC2(sys.SKademliaC2))
	client.SetCredentials(noise.NewCredentials(addr, handshake.NewECDH(), cipher.NewAEAD(), client.Protocol()))

	kv, cleanup := newKV("level", "db")
	ledger := NewLedger(kv, client, nil)
	server := client.Listen()
	RegisterWaveletServer(server, ledger.Protocol())

	go func() {
		if err := server.Serve(ln); err != nil && err != grpc.ErrServerStopped {
			t.Fatal(err)
		}
	}()

	return client, addr, func() {
		server.GracefulStop()
		cleanup()
	}
}

func newKV(kv string, path string) (store.KV, func()) {
	if kv == "inmem" {
		inmemdb := store.NewInmem()
		return inmemdb, func() {
			_ = inmemdb.Close()
		}
	}
	if kv == "level" {
		// Remove existing db
		_ = os.RemoveAll(path)

		leveldb, err := store.NewLevelDB(path)
		if err != nil {
			panic("failed to create LevelDB: " + err.Error())
		}

		return leveldb, func() {
			_ = leveldb.Close()
			_ = os.RemoveAll(path)
		}
	}

	panic("unknown kv " + kv)
}
