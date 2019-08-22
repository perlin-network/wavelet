package wavelet

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/handshake"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

type TestNetwork struct {
	faucet *TestLedger
	nodes  []*TestLedger
}

func NewTestNetwork(t testing.TB) *TestNetwork {
	return &TestNetwork{
		faucet: NewTestFaucet(t),
		nodes:  []*TestLedger{},
	}
}

func (n *TestNetwork) Cleanup() {
	for _, node := range n.nodes {
		node.Cleanup()
	}
	n.faucet.Cleanup()
}

func (n *TestNetwork) Faucet() *TestLedger {
	return n.faucet
}

func (n *TestNetwork) AddNode(t testing.TB, startingBalance uint64) *TestLedger {
	node := NewTestLedger(t, TestLedgerConfig{
		Peers: []string{n.faucet.Addr()},
	})
	n.nodes = append(n.nodes, node)

	if startingBalance > 0 {
		_, err := n.faucet.Pay(node, startingBalance)
		assert.NoError(t, err)
	}

	return node
}

func (n *TestNetwork) WaitForConsensus(t testing.TB) {
	all := append(n.nodes, n.faucet)
	TestWaitForConsensus(t, time.Second*30, all)
}

type TestLedger struct {
	ledger    *Ledger
	server    *grpc.Server
	addr      string
	kv        store.KV
	kvCleanup func()
	finalized chan struct{}
	stopped   chan struct{}
}

type TestLedgerConfig struct {
	Wallet string
	Peers  []string
}

func defaultConfig(t testing.TB) *TestLedgerConfig {
	return &TestLedgerConfig{}
}

// NewTestFaucet returns an account with a lot of PERLs.
func NewTestFaucet(t testing.TB) *TestLedger {
	return NewTestLedger(t, TestLedgerConfig{
		Wallet: "87a6813c3b4cf534b6ae82db9b1409fa7dbd5c13dba5858970b56084c4a930eb400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
	})
}

func NewTestLedger(t testing.TB, cfg TestLedgerConfig) *TestLedger {
	keys := loadKeys(t, cfg.Wallet)

	ln, err := net.Listen("tcp", ":0")
	assert.NoError(t, err)

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))

	client := skademlia.NewClient(addr, keys, skademlia.WithC1(sys.SKademliaC1), skademlia.WithC2(sys.SKademliaC2))
	client.SetCredentials(noise.NewCredentials(addr, handshake.NewECDH(), cipher.NewAEAD(), client.Protocol()))

	kv, cleanup := store.NewTestKV(t, "inmem", "db")
	ledger := NewLedger(kv, client, WithoutGC())
	server := client.Listen()
	RegisterWaveletServer(server, ledger.Protocol())

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if err := server.Serve(ln); err != nil && err != grpc.ErrServerStopped {
			t.Fatal(err)
		}
	}()

	for _, addr := range cfg.Peers {
		if _, err := client.Dial(addr); err != nil {
			t.Fatal(err)
		}
	}

	client.Bootstrap()

	return &TestLedger{
		ledger:    ledger,
		server:    server,
		addr:      addr,
		kv:        kv,
		kvCleanup: cleanup,
		stopped:   stopped,
	}
}

func (l *TestLedger) Cleanup() {
	l.server.GracefulStop()
	<-l.stopped

	//l.kvCleanup()
}

func (l *TestLedger) Addr() string {
	return l.addr
}

func (l *TestLedger) PublicKey() AccountID {
	keys := l.ledger.client.Keys()
	return keys.PublicKey()
}

func (l *TestLedger) Balance() uint64 {
	snapshot := l.ledger.Snapshot()
	balance, _ := ReadAccountBalance(snapshot, l.PublicKey())
	return balance
}

func (l *TestLedger) BalanceOfAccount(node *TestLedger) uint64 {
	snapshot := l.ledger.Snapshot()
	balance, _ := ReadAccountBalance(snapshot, node.PublicKey())
	return balance
}

func (l *TestLedger) Stake() uint64 {
	snapshot := l.ledger.Snapshot()
	stake, _ := ReadAccountStake(snapshot, l.PublicKey())
	return stake
}

func (l *TestLedger) StakeWithPublicKey(key AccountID) uint64 {
	snapshot := l.ledger.Snapshot()
	balance, _ := ReadAccountStake(snapshot, key)
	return balance
}

func (l *TestLedger) StakeOfAccount(node *TestLedger) uint64 {
	snapshot := l.ledger.Snapshot()
	balance, _ := ReadAccountStake(snapshot, node.PublicKey())
	return balance
}

func (l *TestLedger) Reward() uint64 {
	snapshot := l.ledger.Snapshot()
	reward, _ := ReadAccountReward(snapshot, l.PublicKey())
	return reward
}

func (l *TestLedger) WaitForConsensus() <-chan bool {
	ch := make(chan bool)
	go func() {
		start := l.ledger.Rounds().Latest()
		timeout := time.NewTimer(time.Second * 3)
		ticker := time.NewTicker(time.Millisecond * 50)
		for {
			select {
			case <-timeout.C:
				ch <- false
				return

			case <-ticker.C:
				current := l.ledger.Rounds().Latest()
				if current.Index > start.Index {
					ch <- true
					return
				}
			}
		}
	}()

	return ch
}

func (l *TestLedger) Nop() (Transaction, error) {
	keys := l.ledger.client.Keys()
	tx := AttachSenderToTransaction(
		keys,
		NewTransaction(keys, sys.TagNop, nil),
		l.ledger.Graph().FindEligibleParents()...)

	err := l.ledger.AddTransaction(tx)
	return tx, err
}

func (l *TestLedger) Pay(to *TestLedger, amount uint64) (Transaction, error) {
	payload := Transfer{
		Recipient: to.PublicKey(),
		Amount:    amount,
	}

	keys := l.ledger.client.Keys()
	tx := AttachSenderToTransaction(
		keys,
		NewTransaction(keys, sys.TagTransfer, payload.Marshal()),
		l.ledger.Graph().FindEligibleParents()...)

	err := l.ledger.AddTransaction(tx)
	return tx, err
}

func (l *TestLedger) PlaceStake(amount uint64) (Transaction, error) {
	payload := Stake{
		Opcode: sys.PlaceStake,
		Amount: amount,
	}

	keys := l.ledger.client.Keys()
	tx := AttachSenderToTransaction(
		keys,
		NewTransaction(keys, sys.TagStake, payload.Marshal()),
		l.ledger.Graph().FindEligibleParents()...)

	err := l.ledger.AddTransaction(tx)
	return tx, err
}

func (l *TestLedger) WithdrawStake(amount uint64) (Transaction, error) {
	payload := Stake{
		Opcode: sys.WithdrawStake,
		Amount: amount,
	}

	keys := l.ledger.client.Keys()
	tx := AttachSenderToTransaction(
		keys,
		NewTransaction(keys, sys.TagStake, payload.Marshal()),
		l.ledger.Graph().FindEligibleParents()...)

	err := l.ledger.AddTransaction(tx)
	return tx, err
}

func (l *TestLedger) WithdrawReward(amount uint64) (Transaction, error) {
	payload := Stake{
		Opcode: sys.WithdrawReward,
		Amount: amount,
	}

	keys := l.ledger.client.Keys()
	tx := AttachSenderToTransaction(
		keys,
		NewTransaction(keys, sys.TagStake, payload.Marshal()),
		l.ledger.Graph().FindEligibleParents()...)

	err := l.ledger.AddTransaction(tx)
	return tx, err
}

func (l *TestLedger) FindTransaction(t testing.TB, id TransactionID) *Transaction {
	return l.ledger.Graph().FindTransaction(id)
}

func (l *TestLedger) Applied(tx Transaction) bool {
	return tx.Depth <= l.ledger.Graph().RootDepth()
}

func TestWaitForConsensus(t testing.TB, timeout time.Duration, ledgers []*TestLedger) {
	var wg sync.WaitGroup
	for _, l := range ledgers {
		wg.Add(1)
		go func(ledger *TestLedger) {
			defer wg.Done()
			ledger.WaitForConsensus()
		}(l)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	select {
	case <-done:
		return

	case <-timer.C:
		t.Fatal("consensus round took too long")
	}
}

// loadKeys returns a keypair from a wallet string, or generates a new one
// if no wallet is provided.
func loadKeys(t testing.TB, wallet string) *skademlia.Keypair {
	// Generate a keypair if wallet is empty
	if wallet == "" {
		keys, err := skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
		assert.NoError(t, err)
		return keys
	}

	if len(wallet) != hex.EncodedLen(edwards25519.SizePrivateKey) {
		t.Fatal(fmt.Errorf("private key is not of the right length"))
	}

	var privateKey edwards25519.PrivateKey
	n, err := hex.Decode(privateKey[:], []byte(wallet))
	if err != nil {
		t.Fatal(err)
	}

	if n != edwards25519.SizePrivateKey {
		t.Fatal(fmt.Errorf("private key is not of the right length"))
	}

	keys, err := skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		t.Fatal(err)
	}

	return keys
}
