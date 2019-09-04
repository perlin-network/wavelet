package wavelet

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
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
	n := &TestNetwork{
		faucet: newTestFaucet(t),
		nodes:  []*TestLedger{},
	}

	n.nodes = append(n.nodes, n.faucet)
	return n
}

func (n *TestNetwork) Cleanup() {
	for _, node := range n.nodes {
		node.Cleanup()
	}
}

func (n *TestNetwork) Faucet() *TestLedger {
	return n.faucet
}

func (n *TestNetwork) AddNode(t testing.TB) *TestLedger {
	node := NewTestLedger(t, TestLedgerConfig{
		Peers: []string{n.faucet.Addr()},
		N:     len(n.nodes),
	})
	n.nodes = append(n.nodes, node)

	return node
}

func (n *TestNetwork) Nodes() []*TestLedger {
	return n.nodes
}

// WaitForRound waits for all the nodes in the network to
// reach the specified round.
func (n *TestNetwork) WaitForRound(t testing.TB, round uint64) {
	if len(n.nodes) == 0 {
		return
	}

	done := make(chan struct{})
	go func() {
		for _, node := range n.nodes {
			for {
				ri := <-node.WaitForRound(round)
				if ri == round {
					break
				}
			}
		}

		close(done)
	}()

	select {
	case <-time.After(time.Second * 10):
		t.Fatal("timed out waiting for round")

	case <-done:
		return
	}
}

func (n *TestNetwork) WaitForConsensus(t testing.TB) {
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for _, l := range n.nodes {
		wg.Add(1)
		go func(ledger *TestLedger) {
			defer wg.Done()
			for {
				select {
				case c := <-ledger.WaitForConsensus():
					if c {
						return
					}

				case <-stop:
					return
				}
			}

		}(l)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(10 * time.Second)
	select {
	case <-done:
		close(stop)
		return

	case <-timer.C:
		close(stop)
		<-done
		t.Fatal("consensus round took too long")
	}
}

func (n *TestNetwork) WaitForSync(t testing.TB) {
	var wg sync.WaitGroup
	for _, l := range n.nodes {
		wg.Add(1)
		go func(ledger *TestLedger) {
			defer wg.Done()
			assert.True(t, <-ledger.WaitForSync())
		}(l)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(5 * time.Second)
	select {
	case <-done:
		return
	case <-timer.C:
		t.Fatal("timeout while waiting for all nodes to be synced")
	}
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
	N      int
}

func defaultConfig(t testing.TB) *TestLedgerConfig {
	return &TestLedgerConfig{}
}

// newTestFaucet returns an account with a lot of PERLs.
func newTestFaucet(t testing.TB) *TestLedger {
	return NewTestLedger(t, TestLedgerConfig{
		Wallet: "87a6813c3b4cf534b6ae82db9b1409fa7dbd5c13dba5858970b56084c4a930eb400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
		N:      0,
	})
}

func NewTestLedger(t testing.TB, cfg TestLedgerConfig) *TestLedger {
	keys := loadKeys(t, cfg.Wallet)

	ln, err := net.Listen("tcp", ":0")
	assert.NoError(t, err)

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))

	client := skademlia.NewClient(addr, keys, skademlia.WithC1(sys.SKademliaC1), skademlia.WithC2(sys.SKademliaC2))
	client.SetCredentials(noise.NewCredentials(addr, handshake.NewECDH(), cipher.NewAEAD(), client.Protocol()))

	kv, cleanup := store.NewTestKV(t, "level", fmt.Sprintf("db_%d", cfg.N))
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
	l.server.Stop()
	<-l.stopped

	l.kvCleanup()
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

func (l *TestLedger) GasBalanceOfAddress(address [32]byte) uint64 {
	snapshot := l.ledger.Snapshot()
	balance, _ := ReadAccountContractGasBalance(snapshot, address)
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

func (l *TestLedger) RoundIndex() uint64 {
	return l.ledger.Rounds().Latest().Index
}

func (l *TestLedger) WaitForConsensus() <-chan bool {
	ch := make(chan bool)
	go func() {
		start := l.ledger.Rounds().Latest()
		timeout := time.NewTimer(time.Second * 3)
		ticker := time.NewTicker(time.Millisecond * 10)

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

func (l *TestLedger) WaitForRound(index uint64) <-chan uint64 {
	ch := make(chan uint64)
	go func() {
		timeout := time.NewTimer(time.Second * 3)
		ticker := time.NewTicker(time.Millisecond * 10)

		for {
			select {
			case <-timeout.C:
				ch <- 0
				return

			case <-ticker.C:
				current := l.ledger.Rounds().Latest()
				if current.Index >= index {
					ch <- current.Index
					return
				}
			}
		}
	}()

	return ch
}

func (l *TestLedger) WaitForSync() <-chan bool {
	ch := make(chan bool)
	go func() {
		timeout := time.NewTimer(time.Second * 3)
		ticker := time.NewTicker(time.Millisecond * 50)
		for {
			select {
			case <-timeout.C:
				ch <- false
				return

			case <-ticker.C:
				if l.ledger.SyncStatus() == "Node is fully synced" {
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

func (l *TestLedger) SpawnContract(contractPath string, gasLimit uint64, params []byte) (Transaction, error) {
	code, err := ioutil.ReadFile(contractPath)
	if err != nil {
		return Transaction{}, err
	}

	payload := Contract{
		GasLimit: gasLimit,
		Code:     code,
		Params:   params,
	}

	keys := l.ledger.client.Keys()
	tx := AttachSenderToTransaction(
		keys,
		NewTransaction(keys, sys.TagContract, payload.Marshal()),
		l.ledger.Graph().FindEligibleParents()...)

	err = l.ledger.AddTransaction(tx)
	return tx, err
}

func (l *TestLedger) DepositGas(id [32]byte, gasDeposit uint64) (Transaction, error) {
	payload := Transfer{
		Recipient:  id,
		GasDeposit: gasDeposit,
	}

	keys := l.ledger.client.Keys()
	tx := AttachSenderToTransaction(
		keys,
		NewTransaction(keys, sys.TagTransfer, payload.Marshal()),
		l.ledger.Graph().FindEligibleParents()...)

	err := l.ledger.AddTransaction(tx)
	return tx, err
}

func (l *TestLedger) CallContract(id [32]byte, amount uint64, gasLimit uint64, funcName string, params []byte) (Transaction, error) {
	payload := Transfer{
		Recipient:  id,
		Amount:     amount,
		GasLimit:   gasLimit,
		FuncName:   []byte(funcName),
		FuncParams: params,
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

func waitFor(t testing.TB, err string, fn func() bool) {
	timeout := time.NewTimer(time.Second * 10)
	ticker := time.NewTicker(time.Millisecond * 500)

	for {
		select {
		case <-timeout.C:
			t.Fatal(err)

		case <-ticker.C:
			if fn() {
				return
			}
		}
	}
}
