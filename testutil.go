// +build unit integration

package wavelet

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/handshake"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"google.golang.org/grpc"
)

var (
	FaucetWallet = "87a6813c3b4cf534b6ae82db9b1409fa7dbd5c13dba5858970b56084c4a930eb400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405"
)

type TestNetwork struct {
	faucet *TestLedger
	nodes  map[AccountID]*TestLedger
}

type TestNetworkConfig struct {
	AddFaucet bool
}

func defaultTestNetworkConfig() TestNetworkConfig {
	return TestNetworkConfig{
		AddFaucet: true,
	}
}

type TestNetworkOption func(cfg *TestNetworkConfig)

func NewTestNetwork(opts ...TestNetworkOption) (*TestNetwork, error) {
	n := &TestNetwork{
		nodes: map[AccountID]*TestLedger{},
	}

	cfg := defaultTestNetworkConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	var err error
	if cfg.AddFaucet {
		n.faucet, err = n.AddNode(WithWallet(FaucetWallet), WithRemoveExistingDB(true))
		if err != nil {
			return nil, err
		}
	}

	return n, nil
}

func (n *TestNetwork) Cleanup() {
	for _, node := range n.nodes {
		node.Cleanup(true)
	}
}

func (n *TestNetwork) Faucet() *TestLedger {
	return n.faucet
}

func (n *TestNetwork) SetFaucet(node *TestLedger) {
	n.faucet = node
}

type TestLedgerOption func(cfg *TestLedgerConfig)

func WithWallet(wallet string) TestLedgerOption {
	return func(cfg *TestLedgerConfig) {
		cfg.Wallet = wallet
	}
}

func WithRemoveExistingDB(remove bool) TestLedgerOption {
	return func(cfg *TestLedgerConfig) {
		cfg.RemoveExistingDB = remove
	}
}

func WithDBPath(path string) TestLedgerOption {
	return func(cfg *TestLedgerConfig) {
		cfg.DBPath = path
	}
}

func (n *TestNetwork) AddNode(opts ...TestLedgerOption) (*TestLedger, error) {
	var peers []string

	if n.faucet != nil {
		peers = append(peers, n.faucet.Addr())
	}

	cfg := TestLedgerConfig{
		Peers: peers,
		N:     len(n.nodes),
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	node, err := NewTestLedger(cfg)
	if err != nil {
		return nil, err
	}

	node.network = n
	n.nodes[node.PublicKey()] = node

	return node, nil
}

func (n *TestNetwork) Nodes() []*TestLedger {
	nodes := make([]*TestLedger, 0, len(n.nodes))
	for _, n := range n.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

// WaitForRound waits for all the nodes in the network to
// reach the specified block.
func (n *TestNetwork) WaitForBlock(block uint64) error {
	if len(n.nodes) == 0 {
		return nil
	}

	results := make(chan error)

	for _, node := range n.nodes {
		go func() {
			if ri := <-node.WaitForBlock(block); ri != block {
				results <- fmt.Errorf("for %x block expected to be %d but got %d", node.PublicKey(), block, ri)
			} else {
				results <- nil
			}
		}()
	}

	for range n.nodes {
		if err := <-results; err != nil {
			return err
		}
	}

	return nil
}

func (n *TestNetwork) WaitForConsensus() error {
	results := make(chan error)

	for _, l := range n.nodes {
		go func(ledger *TestLedger) {
			err := <-ledger.WaitForConsensus()
			results <- err
		}(l)
	}

	for range n.nodes {
		if err := <-results; err != nil {
			return err
		}
	}

	return nil
}

func (n *TestNetwork) WaitForSync() error {
	syncs := make(chan error)
	for _, l := range n.nodes {
		go func(ledger *TestLedger) {
			syncedErr := <-ledger.WaitForSync()
			syncs <- syncedErr
		}(l)
	}

	for range n.nodes {
		if syncedErr := <-syncs; syncedErr != nil {
			return syncedErr
		}
	}

	return nil
}

func (n *TestNetwork) WaitUntilSync() error {
	for _, l := range n.nodes {
		if err := l.WaitUntilSync(); err != nil {
			return err
		}
	}

	return nil
}

type TestLedger struct {
	network   *TestNetwork
	nonce     uint64
	ledger    *Ledger
	client    *skademlia.Client
	server    *grpc.Server
	addr      string
	dbPath    string
	kv        store.KV
	kvCleanup func()
	stopped   chan struct{}

	synced bool
}

type TestLedgerConfig struct {
	Wallet           string
	Peers            []string
	N                int
	RemoveExistingDB bool
	DBPath           string
}

func NewTestLedger(cfg TestLedgerConfig) (*TestLedger, error) {
	keys, err := loadKeys(cfg.Wallet)
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", ":0") // nolint:gosec
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))

	client := skademlia.NewClient(addr, keys, skademlia.WithC1(sys.SKademliaC1), skademlia.WithC2(sys.SKademliaC2))
	client.SetCredentials(noise.NewCredentials(addr, handshake.NewECDH(), cipher.NewAEAD(), client.Protocol()))

	var kvOpts []store.TestKVOption
	if !cfg.RemoveExistingDB {
		kvOpts = append(kvOpts, store.WithKeepExisting())
	}

	path := fmt.Sprintf("db_%d", cfg.N)
	if cfg.DBPath != "" {
		path = cfg.DBPath
	}

	kv, cleanup, err := store.NewTestKV("level", path, kvOpts...)
	if err != nil {
		return nil, err
	}

	ledger, err := NewLedger(kv, client, WithoutGC())
	if err != nil {
		return nil, err
	}

	server := client.Listen()
	RegisterWaveletServer(server, ledger.Protocol())

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if err := server.Serve(ln); err != nil && err != grpc.ErrServerStopped {
			fmt.Println(err)
		}
	}()

	for _, addr := range cfg.Peers {
		if _, err := client.Dial(addr); err != nil {
			return nil, err
		}
	}

	client.Bootstrap()

	tl := &TestLedger{
		ledger:    ledger,
		client:    client,
		server:    server,
		addr:      addr,
		dbPath:    path,
		kv:        kv,
		kvCleanup: cleanup,
		stopped:   stopped,
	}

	tl.ledger.syncManager.OnSynced = append(tl.ledger.syncManager.OnSynced, func(block Block) {
		tl.synced = true
	})

	return tl, nil
}

func (l *TestLedger) Leave(wipeDB bool) {
	l.Cleanup(wipeDB)
	delete(l.network.nodes, l.PublicKey())
}

func (l *TestLedger) Cleanup(wipeDB bool) {
	l.server.Stop()
	<-l.stopped

	l.ledger.Close()
	l.kvCleanup()

	if wipeDB && len(l.DBPath()) != 0 {
		if err := os.RemoveAll(l.DBPath()); err != nil {
			panic(err)
		}
	}
}

func (l *TestLedger) Addr() string {
	return l.addr
}

func (l *TestLedger) Ledger() *Ledger {
	return l.ledger
}

func (l *TestLedger) Client() *skademlia.Client {
	return l.client
}

func (l *TestLedger) KV() store.KV {
	return l.kv
}

func (l *TestLedger) Keys() *skademlia.Keypair {
	return l.ledger.client.Keys()
}

func (l *TestLedger) PrivateKey() edwards25519.PrivateKey {
	keys := l.ledger.client.Keys()
	return keys.PrivateKey()
}

func (l *TestLedger) PublicKey() AccountID {
	keys := l.ledger.client.Keys()
	return keys.PublicKey()
}

func (l *TestLedger) DBPath() string {
	return l.dbPath
}

func (l *TestLedger) Balance() uint64 {
	snapshot := l.ledger.Snapshot()
	balance, _ := ReadAccountBalance(snapshot, l.PublicKey())
	return balance
}

func (l *TestLedger) BalanceWithPublicKey(key AccountID) uint64 {
	snapshot := l.ledger.Snapshot()
	balance, _ := ReadAccountBalance(snapshot, key)
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
	stake, _ := ReadAccountStake(snapshot, node.PublicKey())
	return stake
}

func (l *TestLedger) Reward() uint64 {
	snapshot := l.ledger.Snapshot()
	reward, _ := ReadAccountReward(snapshot, l.PublicKey())
	return reward
}

func (l *TestLedger) RewardWithPublicKey(key AccountID) uint64 {
	snapshot := l.ledger.Snapshot()
	reward, _ := ReadAccountReward(snapshot, key)
	return reward
}

func (l *TestLedger) BlockIndex() uint64 {
	return l.ledger.Blocks().Latest().Index
}

func (l *TestLedger) WaitForConsensus() <-chan error {
	ch := make(chan error)
	go func() {
		start := l.ledger.Blocks().Latest()
		timeout := time.NewTimer(time.Second * 10)
		ticker := time.NewTicker(time.Millisecond * 5)

		for {
			select {
			case <-timeout.C:
				ch <- fmt.Errorf("%x has not proceed to next block", l.PublicKey())
				return

			case <-ticker.C:
				current := l.ledger.Blocks().Latest()
				if current.Index > start.Index {
					ch <- nil
					return
				}
			}
		}
	}()

	return ch
}

func (l *TestLedger) WaitUntilConsensus() error {
	return <-l.WaitForConsensus()
}

// WaitUntilBalance should be used to ensure that the ledger's balance
// is of a specific value before continuing.
func (l *TestLedger) WaitUntilBalance(balance uint64) error {
	ticker := time.NewTicker(time.Millisecond * 200)
	timeout := time.NewTimer(time.Second * 10)
	for {
		select {
		case <-ticker.C:
			if l.Balance() == balance {
				return nil
			}
		case <-timeout.C:
			return errors.New("timed out waiting for balance")
		}
	}
}

func (l *TestLedger) WaitUntilStake(stake uint64) error {
	ticker := time.NewTicker(time.Millisecond * 200)
	timeout := time.NewTimer(time.Second * 10)

	for {
		select {
		case <-ticker.C:
			if l.Stake() == stake {
				return nil
			}
		case <-timeout.C:
			return errors.New("timed out waiting for stake")
		}
	}
}

func (l *TestLedger) WaitForBlock(index uint64) <-chan uint64 {
	ch := make(chan uint64)
	go func() {
		timeout := time.NewTimer(time.Second * 10)
		ticker := time.NewTicker(time.Millisecond * 10)

		for {
			select {
			case <-timeout.C:
				ch <- 0
				return

			case <-ticker.C:
				current := l.ledger.Blocks().Latest()
				if current.Index >= index {
					ch <- current.Index
					return
				}
			}
		}
	}()

	return ch
}

func (l *TestLedger) WaitUntilBlock(block uint64) error {
	timeout := time.NewTimer(time.Second * 300)
	for {
		select {
		case ri := <-l.WaitForBlock(block):
			if ri >= block {
				return nil
			}
		case <-timeout.C:
			return errors.New("timed out waiting for block")
		}
	}
}

func (l *TestLedger) WaitForSync() <-chan error {
	l.synced = false

	ch := make(chan error)
	go func() {
		timeout := time.NewTimer(time.Second * 30)
		timer := time.NewTicker(50 * time.Millisecond)

		defer timeout.Stop()
		defer timer.Stop()

		for {
			select {
			case <-timeout.C:
				ch <- fmt.Errorf("%x timed out waiting for sync", l.PublicKey())
				return

			case <-timer.C:
				if l.synced {
					ch <- nil
					return
				}

			}
		}
	}()

	return ch
}

func (l *TestLedger) WaitUntilSync() error {
	return <-l.WaitForSync()
}

func (l *TestLedger) newSignedTransaction(tag sys.Tag, payload []byte) Transaction {
	nonce := atomic.AddUint64(&l.nonce, 1)
	block := l.BlockIndex()

	var nonceBuf [8]byte
	binary.BigEndian.PutUint64(nonceBuf[:], nonce)

	var blockBuf [8]byte
	binary.BigEndian.PutUint64(blockBuf[:], block)

	keys := l.ledger.client.Keys()
	signature := edwards25519.Sign(
		keys.PrivateKey(),
		append(nonceBuf[:], append(blockBuf[:], append([]byte{byte(tag)}, payload...)...)...),
	)

	return NewSignedTransaction(
		keys.PublicKey(), nonce, block,
		tag, payload, signature,
	)
}

func (l *TestLedger) Pay(to *TestLedger, amount uint64) (Transaction, error) {
	t := Transfer{
		Recipient: to.PublicKey(),
		Amount:    amount,
	}

	var tx Transaction
	payload, err := t.Marshal()
	if err != nil {
		return tx, err
	}

	tx = l.newSignedTransaction(sys.TagTransfer, payload)
	l.ledger.AddTransaction(tx)

	return tx, nil
}

func (l *TestLedger) SpawnContract(contractPath string, gasLimit uint64, params []byte) (Transaction, error) {
	code, err := ioutil.ReadFile(contractPath)
	if err != nil {
		return Transaction{}, err
	}

	c := Contract{
		GasLimit: gasLimit,
		Code:     code,
		Params:   params,
	}

	var tx Transaction
	payload, err := c.Marshal()
	if err != nil {
		return tx, err
	}

	tx = l.newSignedTransaction(sys.TagContract, payload)
	l.ledger.AddTransaction(tx)

	return tx, nil
}

func (l *TestLedger) DepositGas(id [32]byte, gasDeposit uint64) (Transaction, error) {
	t := Transfer{
		Recipient:  id,
		GasDeposit: gasDeposit,
	}

	var tx Transaction
	payload, err := t.Marshal()
	if err != nil {
		return tx, err
	}

	tx = l.newSignedTransaction(sys.TagTransfer, payload)
	l.ledger.AddTransaction(tx)

	return tx, nil
}

func (l *TestLedger) CallContract(id [32]byte, amount uint64, gasLimit uint64, funcName string, params []byte) (Transaction, error) {
	t := Transfer{
		Recipient:  id,
		Amount:     amount,
		GasLimit:   gasLimit,
		FuncName:   []byte(funcName),
		FuncParams: params,
	}

	var tx Transaction
	payload, err := t.Marshal()
	if err != nil {
		return tx, err
	}

	tx = l.newSignedTransaction(sys.TagTransfer, payload)
	l.ledger.AddTransaction(tx)

	return tx, nil
}

func (l *TestLedger) PlaceStake(amount uint64) (Transaction, error) {
	s := Stake{
		Opcode: sys.PlaceStake,
		Amount: amount,
	}

	var tx Transaction
	payload, err := s.Marshal()
	if err != nil {
		return tx, err
	}

	tx = l.newSignedTransaction(sys.TagStake, payload)
	l.ledger.AddTransaction(tx)

	return tx, nil
}

func (l *TestLedger) WithdrawStake(amount uint64) (Transaction, error) {
	s := Stake{
		Opcode: sys.WithdrawStake,
		Amount: amount,
	}

	var tx Transaction
	payload, err := s.Marshal()
	if err != nil {
		return tx, err
	}

	tx = l.newSignedTransaction(sys.TagStake, payload)
	l.ledger.AddTransaction(tx)

	return tx, nil
}

// loadKeys returns a keypair from a wallet string, or generates a new one
// if no wallet is provided.
func loadKeys(wallet string) (*skademlia.Keypair, error) {
	// Generate a keypair if wallet is empty
	if wallet == "" {
		return skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	}

	if len(wallet) != hex.EncodedLen(edwards25519.SizePrivateKey) {
		return nil, fmt.Errorf("private key is not of the right length")
	}

	var privateKey edwards25519.PrivateKey
	n, err := hex.Decode(privateKey[:], []byte(wallet))
	if err != nil {
		return nil, err
	}

	if n != edwards25519.SizePrivateKey {
		return nil, fmt.Errorf("private key is not of the right length")
	}

	keys, err := skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		return nil, err
	}

	return keys, nil
}

func waitFor(fn func() bool) error {
	timeout := time.NewTimer(time.Second * 30)
	ticker := time.NewTicker(time.Millisecond * 100)

	for {
		select {
		case <-timeout.C:
			return errors.New("timed out waiting")
		case <-ticker.C:
			if fn() {
				return nil
			}
		}
	}
}

func FailTest(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
