// +build integration

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/log"
	"github.com/phayes/freeport"
	"github.com/stretchr/testify/assert"
)

var wallet1 = "87a6813c3b4cf534b6ae82db9b1409fa7dbd5c13dba5858970b56084c4a930eb400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405"
var wallet2 = "85e7450f7cf0d9cd1d1d7bf4169c2f364eea4ba833a7280e0f931a1d92fd92c2696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a"

func TestMain_Basic(t *testing.T) {
	w, err := NewTestWavelet(defaultConfig())
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	ledger, err := w.GetLedgerStatus()
	wavelet.FailTest(t, err)

	assert.EqualValues(t, "127.0.0.1:"+w.Port, ledger.Address)
	assert.NotEqual(t, wallet1[64:], ledger.PublicKey) // A random wallet should be generated
}

func TestMain_WithLogLevel(t *testing.T) {
	config := defaultConfig()
	config.LogLevel = "warn"
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	w.Stdin <- "status"
	// Info message should not be logged to stdout
	search := "Here is the current status of your node"

	timeout := time.NewTimer(time.Millisecond * 200)
	for {
		select {
		case line := <-w.Stdout.Lines:
			if strings.Contains(line, search) {
				t.Fatal("info should not be logged to stdout")
			}

		case <-timeout.C:
			return
		}
	}
}

func TestMain_WithDefaultLogLevel(t *testing.T) {
	// Test default loglevel should be debug
	config := defaultConfig()
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	w.Stdin <- "status"
	_, err = w.Stdout.Search("Here is the current status of your node", 1)
	assert.NoError(t, err)
}

func TestMain_WithWalletString(t *testing.T) {
	config := defaultConfig()
	config.Wallet = "b27b880e6e44e3b127186a08bc5698316e8dd99157cec56211560b62141f0851c72096021609681eb8cab244752945b2008e1b51d8bc2208b2b562f35485d5cc"
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	ledger, err := w.GetLedgerStatus()
	wavelet.FailTest(t, err)

	assert.EqualValues(t, config.Wallet[64:], ledger.PublicKey)
}

func TestMain_WithWalletFile(t *testing.T) {
	wallet := "d6acf5caca96e9da2088bc3a051ed938749145c3f3f7b5bd81aefeb46c12f9e901e4d5b4f51ef76a9a9cc511af910964943404172347b6a01bcfe65d768c9354"

	// Write wallet to a temporary file
	dir, err := ioutil.TempDir("", "wavelet")
	wavelet.FailTest(t, err)

	defer func() {
		_ = os.RemoveAll(dir)
	}()

	walletPath := filepath.Join(dir, "wallet.txt")
	err = ioutil.WriteFile(walletPath, []byte(wallet), 0666)
	wavelet.FailTest(t, err)

	config := defaultConfig()
	config.Wallet = walletPath
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	ledger, err := w.GetLedgerStatus()
	wavelet.FailTest(t, err)

	assert.EqualValues(t, wallet[64:], ledger.PublicKey)
}

func TestMain_WithInvalidWallet(t *testing.T) {
	config := defaultConfig()
	config.Wallet = "foobar"
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	ledger, err := w.GetLedgerStatus()
	wavelet.FailTest(t, err)

	assert.NotEqual(t, wallet1[64:], ledger.PublicKey) // A random wallet should be generated
}

func TestMain_Status(t *testing.T) {
	w, err := NewTestWavelet(defaultConfig())
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	w.Stdin <- "status"
	_, err = w.Stdout.Search("Here is the current status of your node", 1)
	assert.NoError(t, err)
}

func TestMain_Pay(t *testing.T) {
	config := defaultConfig()
	config.Wallet = wallet2
	alice, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer alice.Cleanup()

	bob, err := alice.Testnet.AddNode()
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, alice.Testnet.WaitForSync())

	recipient := bob.PublicKey()
	alice.Stdin <- fmt.Sprintf("p %s 99999", hex.EncodeToString(recipient[:]))

	txIDText, err := alice.Stdout.Search("Transfer sent.", 1)
	wavelet.FailTest(t, err)

	txID, err := extractTxID(txIDText)
	wavelet.FailTest(t, err)

	tx, err := alice.WaitForTransaction(txID)
	wavelet.FailTest(t, err)

	assert.EqualValues(t, txID, tx.ID)
	assert.EqualValues(t, alice.PublicKey, tx.Sender)

	assert.NoError(t, bob.WaitUntilBalance(99999))
}

func TestMain_Spawn(t *testing.T) {
	config := defaultConfig()
	config.Wallet = wallet2
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	_, err = w.Testnet.AddNode()
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, w.Testnet.WaitForSync())

	w.Stdin <- "spawn ../../testdata/transfer_back.wasm"

	txIDText, err := w.Stdout.Search("Contract sent.", 1)
	wavelet.FailTest(t, err)

	txID, err := extractTxID(txIDText)
	wavelet.FailTest(t, err)

	tx, err := w.WaitForTransaction(txID)
	wavelet.FailTest(t, err)

	assert.EqualValues(t, txID, tx.ID)
	assert.EqualValues(t, w.PublicKey, tx.Sender)
}

func TestMain_Call(t *testing.T) {
	config := defaultConfig()
	config.Wallet = wallet2
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	_, err = w.Testnet.AddNode()
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, w.Testnet.WaitForSync())

	w.Stdin <- "spawn ../../testdata/transfer_back.wasm"

	contractIDText, err := w.Stdout.Search("Contract sent.", 1)
	wavelet.FailTest(t, err)

	txID, err := extractTxID(contractIDText)
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, w.WaitForConsensus(3))

	tx, err := w.WaitForTransaction(txID)
	wavelet.FailTest(t, err)

	w.Stdin <- fmt.Sprintf("call %s 1000 100000 on_money_received", tx.ID)

	_, err = w.Stdout.Search("Transfer sent.", 1)
	assert.NoError(t, err)
}

func TestMain_CallWithParams(t *testing.T) {
	config := defaultConfig()
	config.Wallet = wallet2
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	_, err = w.Testnet.AddNode()
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, w.Testnet.WaitForSync())

	w.Stdin <- "spawn ../../testdata/transfer_back.wasm"

	txIDText, err := w.Stdout.Search("Contract sent.", 1)
	wavelet.FailTest(t, err)

	txID, err := extractTxID(txIDText)
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, w.WaitForConsensus(3))

	tx, err := w.WaitForTransaction(txID)
	wavelet.FailTest(t, err)

	params := "Sfoobar Bloremipsum 142 21337 4666666 831415926535 Hbada55"

	w.Stdin <- fmt.Sprintf("call %s 1000 100000 on_money_received %s", tx.ID, params)

	txIDText, err = w.Stdout.Search("Transfer sent.", 1)
	wavelet.FailTest(t, err)

	txID, err = extractTxID(txIDText)
	wavelet.FailTest(t, err)

	tx, err = w.WaitForTransaction(txID)
	wavelet.FailTest(t, err)

	encodedParams, err := base64.StdEncoding.DecodeString(tx.Payload)
	if err != nil {
		t.Fatal(err)
	}

	encodedParams = encodedParams[wavelet.SizeAccountID:]
	encodedParams = encodedParams[8+8+8+4:] // Amount, gas limit, gas deposit
	encodedParams = encodedParams[len("on_money_received"):]
	encodedParams = encodedParams[4:] // Params length

	hexParams := hex.EncodeToString(encodedParams)

	// foobar
	buf := hexParams[:14]
	assert.EqualValues(t, hex.EncodeToString(append([]byte("foobar"), byte(0))), buf)
	hexParams = hexParams[14:]

	// loremipsum
	buf = hexParams[:28]
	assert.EqualValues(t, "0a000000", buf[:8])
	assert.EqualValues(t, hex.EncodeToString([]byte("loremipsum")), buf[8:])
	hexParams = hexParams[28:]

	// 42
	buf = hexParams[:2]
	assert.EqualValues(t, "2a", buf[:2])
	hexParams = hexParams[2:]

	// 1337
	buf = hexParams[:4]
	assert.EqualValues(t, "3905", buf[:4])
	hexParams = hexParams[4:]

	// 666666
	buf = hexParams[:8]
	assert.EqualValues(t, "2a2c0a00", buf[:8])
	hexParams = hexParams[8:]

	// 31415926535
	buf = hexParams[:16]
	assert.EqualValues(t, "07ff885007000000", buf[:16])
	hexParams = hexParams[16:]

	// 0xbada55
	buf = hexParams
	assert.EqualValues(t, "bada55", buf)
}

func TestMain_DepositGas(t *testing.T) {
	config := defaultConfig()
	config.Wallet = wallet2
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	_, err = w.Testnet.AddNode()
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, w.Testnet.WaitForSync())

	w.Stdin <- "spawn ../../testdata/transfer_back.wasm"

	txIDText, err := w.Stdout.Search("Contract sent.", 1)
	wavelet.FailTest(t, err)

	txID, err := extractTxID(txIDText)
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, w.WaitForConsensus(3))

	tx, err := w.WaitForTransaction(txID)
	wavelet.FailTest(t, err)

	w.Stdin <- fmt.Sprintf("deposit-gas %s 99999", tx.ID)

	_, err = w.Stdout.Search("Transfer sent.", 1)
	assert.NoError(t, err)
}

func TestMain_Find(t *testing.T) {
	config := defaultConfig()
	config.Wallet = wallet2
	alice, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer alice.Cleanup()

	bob, err := alice.Testnet.AddNode()
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, alice.Testnet.WaitForSync())

	recipient := bob.PublicKey()
	alice.Stdin <- fmt.Sprintf("p %s 99999", hex.EncodeToString(recipient[:]))

	txIDText, err := alice.Stdout.Search("Transfer sent.", 1)
	wavelet.FailTest(t, err)

	txID, err := extractTxID(txIDText)
	wavelet.FailTest(t, err)

	_, err = alice.WaitForTransaction(txID)
	wavelet.FailTest(t, err)

	alice.Stdin <- fmt.Sprintf("find %s", txID)

	_, err = alice.Stdout.Search(fmt.Sprintf("Transaction id: %s", txID), 1)
	assert.NoError(t, err)
}

func TestMain_PlaceStake(t *testing.T) {
	config := defaultConfig()
	config.Wallet = wallet2
	alice, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer alice.Cleanup()

	bob, err := alice.Testnet.AddNode()
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, alice.Testnet.WaitForSync())

	alice.Stdin <- "ps 1000"

	txIDText, err := alice.Stdout.Search("Stake changed.", 1)
	wavelet.FailTest(t, err)

	txID, err := extractTxID(txIDText)
	wavelet.FailTest(t, err)

	tx, err := alice.WaitForTransaction(txID)
	wavelet.FailTest(t, err)

	assert.EqualValues(t, txID, tx.ID)
	assert.EqualValues(t, alice.PublicKey, tx.Sender)

	err = waitFor(func() error {
		accID, err := asAccountID(alice.PublicKey)
		if err != nil {
			return err
		}

		if bob.StakeWithPublicKey(accID) != 1000 {
			return fmt.Errorf("wrong stake amount")
		}

		return nil
	})

	assert.NoError(t, err)
}

func TestMain_WithdrawStake(t *testing.T) {
	config := defaultConfig()
	config.Wallet = wallet2
	alice, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer alice.Cleanup()

	bob, err := alice.Testnet.AddNode()
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, alice.Testnet.WaitForSync())

	alice.Stdin <- "ps 1000"

	txIDText, err := alice.Stdout.Search("Stake changed.", 1)
	wavelet.FailTest(t, err)

	txID, err := extractTxID(txIDText)
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, alice.WaitForConsensus(3))

	alice.Stdin <- "ws 500"

	txIDText, err = alice.Stdout.Search("Stake changed.", 1)
	wavelet.FailTest(t, err)

	txID, err = extractTxID(txIDText)
	wavelet.FailTest(t, err)

	tx, err := alice.WaitForTransaction(txID)
	wavelet.FailTest(t, err)

	assert.EqualValues(t, txID, tx.ID)
	assert.EqualValues(t, alice.PublicKey, tx.Sender)

	err = waitFor(func() error {
		accID, err := asAccountID(alice.PublicKey)
		if err != nil {
			return err
		}

		if bob.StakeWithPublicKey(accID) != 500 {
			return fmt.Errorf("wrong stake amount")
		}

		return nil
	})

	assert.NoError(t, err)
}

func TestMain_WithdrawReward(t *testing.T) {
	config := defaultConfig()
	config.Wallet = wallet2
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	_, err = w.Testnet.AddNode()
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, w.Testnet.WaitForSync())

	w.Stdin <- "wr 1000"

	txIDText, err := w.Stdout.Search("Stake changed.", 1)
	wavelet.FailTest(t, err)

	txID, err := extractTxID(txIDText)
	wavelet.FailTest(t, err)

	tx, err := w.WaitForTransaction(txID)
	wavelet.FailTest(t, err)

	assert.EqualValues(t, txID, tx.ID)
	assert.EqualValues(t, w.PublicKey, tx.Sender)

	// TODO: check if reward is actually withdrawn
}

func TestMain_UpdateParams(t *testing.T) {
	w, err := NewTestWavelet(defaultConfig())
	wavelet.FailTest(t, err)

	defer func() {
		w.Cleanup()
		conf.Reset()
	}()

	w.Stdin <- "up"

	_, err = w.Stdout.Search("Current configuration values", 1)
	wavelet.FailTest(t, err)

	tests := []struct {
		Config string
		Var    string
		Value  interface{}
	}{
		{"snowball.k", "snowballK", 123},
		{"snowball.beta", "snowballBeta", 789},
		{"vote.sync.threshold", "syncVoteThreshold", 12.34},
		{"vote.finalization.threshold", "finalizationVoteThreshold", 56.78},
		{"vote.finalization.stake.weight", "stakeMajorityWeight", 11.11},
		{"query.timeout", "queryTimeout", time.Second * 9},
		{"gossip.timeout", "gossipTimeout", time.Second * 4},
		{"download.tx.timeout", "downloadTxTimeout", time.Second * 3},
		{"check.out.of.sync.timeout", "checkOutOfSyncTimeout", time.Second * 7},
		{"sync.chunk.size", "syncChunkSize", 1337},
		{"sync.if.block.indices.differ.by", "syncIfBlockIndicesDifferBy", uint64(42)},
		{"pruning.limit", "pruningLimit", uint64(255)},
		{"api.secret", "secret", "shambles"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.Config, func(t *testing.T) {
			w.Stdin <- fmt.Sprintf("up --%s %+v", tt.Config, tt.Value)

			searchVal := tt.Value
			switch v := tt.Value.(type) {
			case time.Duration:
				searchVal = strconv.FormatUint(uint64(v), 10)
			case int:
				searchVal = strconv.Itoa(v)
			case uint64:
				searchVal = strconv.FormatUint(v, 10)
			case float64:
				searchVal = strconv.FormatFloat(v, 'f', -1, 64)
			case string:
				searchVal = v
			}

			_, err = w.Stdout.Search(fmt.Sprintf("%s:%s", tt.Var, searchVal), 1)
			wavelet.FailTest(t, err)
		})
	}
}

func TestMain_ConnectDisconnect(t *testing.T) {
	config := defaultConfig()
	config.Wallet = wallet2
	w, err := NewTestWavelet(config)
	wavelet.FailTest(t, err)

	defer w.Cleanup()

	_, err = w.Testnet.AddNode()
	wavelet.FailTest(t, err)

	wavelet.FailTest(t, w.Testnet.WaitForSync())

	peerAddr := w.Testnet.Nodes()[0].Addr()
	w.Stdin <- fmt.Sprintf("connect %s", peerAddr)
	_, err = w.Stdout.Search("Successfully connected to", 1)
	wavelet.FailTest(t, err)

	w.Stdin <- fmt.Sprintf("disconnect %s", peerAddr)
	_, err = w.Stdout.Search("Successfully disconnected", 1)

	assert.NoError(t, err)
}

func nextPort() (string, error) {
	port, err := freeport.GetFreePort()
	if err != nil {
		return "", err
	}

	return strconv.Itoa(port), nil
}

func waitForAPI(apiPort string) error {
	timeout := time.NewTimer(time.Second * 30)
	tick := time.NewTicker(time.Second * 1)

	for {
		select {
		case <-timeout.C:
			return errors.New("timed out waiting for API")

		case <-tick.C:
			if _, err := getLedgerStatus(apiPort); err == nil {
				return nil
			}
		}
	}
}

type TestLedgerStatus struct {
	PublicKey string     `json:"public_key"`
	Address   string     `json:"address"`
	Peers     []TestPeer `json:"peers"`
}

type TestPeer struct {
	Address   string `json:"address"`
	PublicKey string `json:"public_key"`
}

type TestTransaction struct {
	ID      string `json:"id"`
	Sender  string `json:"sender"`
	Payload string `json:"payload"`
}

type mockStdin chan string

func (s mockStdin) Read(dst []byte) (n int, err error) {
	line := <-s
	if line == "" {
		return 0, io.EOF
	}

	copy(dst, line+"\n")

	return len(line) + 1, nil
}

func (s mockStdin) Close() error {
	close(s)
	return nil
}

type mockStdout struct {
	Lines chan string
	buf   []byte
	lock  sync.Mutex
}

func newMockStdout() *mockStdout {
	return &mockStdout{
		Lines: make(chan string, 1024),
		buf:   make([]byte, 0),
	}
}

func (s *mockStdout) Write(p []byte) (n int, err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.buf = append(s.buf, p...)

	ni := bytes.Index(s.buf, []byte{'\n'})
	if ni < 0 {
		return len(p), nil
	}

	for ni >= 0 {
		str := string(s.buf[:ni])
		s.Lines <- str

		if len(s.buf) > ni {
			// Try to find the next newline.
			s.buf = s.buf[ni+1:]
			ni = bytes.Index(s.buf, []byte{'\n'})
		} else {
			// The buffer is empty
			s.buf = s.buf[0:]
			break
		}
	}

	return len(p), nil
}

var _stripANSISeq = regexp.MustCompile("\033\\[[0-9;]*m")

func stripANSISeq(input string) string {
	return _stripANSISeq.ReplaceAllString(input, "")
}

func (s *mockStdout) Search(sub string, times int) (string, error) {
	timeout := time.NewTimer(time.Second * 30)
	c := 0
	for {
		select {
		case line := <-s.Lines:
			line = stripANSISeq(line) // clear the color codes

			if strings.Contains(line, sub) {
				c++
				if c == times {
					return line, nil
				}
			}

		case <-timeout.C:
			return "", fmt.Errorf("timed out searching for string in stdout: %q", sub)
		}
	}
}

func extractTxID(s string) (string, error) {
	if len(s) < 64 {
		return "", errors.New("output does not contain tx id")
	}

	txID := s[len(s)-64:]
	if _, err := hex.DecodeString(txID); err != nil {
		return "", err
	}

	return txID, nil
}

func waitFor(fn func() error) error {
	timeout := time.NewTimer(time.Second * 30)
	ticker := time.NewTicker(time.Second * 1)

	for {
		select {
		case <-timeout.C:
			return errors.New("timed out waiting")
		case <-ticker.C:
			if err := fn(); err == nil {
				return nil
			}
		}
	}
}

type TestWavelet struct {
	Testnet   *wavelet.TestNetwork
	Port      string
	APIPort   string
	PublicKey string
	Stdin     mockStdin
	Stdout    *mockStdout
	StopWG    sync.WaitGroup
}

func (w *TestWavelet) Cleanup() {
	w.Testnet.Cleanup()
	close(w.Stdin)
	w.StopWG.Wait()
}

type TestWaveletConfig struct {
	Wallet   string
	LogLevel string
}

func defaultConfig() *TestWaveletConfig {
	return &TestWaveletConfig{
		LogLevel: "info",
	}
}

func NewTestWavelet(cfg *TestWaveletConfig) (*TestWavelet, error) {
	// We set the loglevel directly instead of using the flag, to prevent race condition.
	// The race condition will happen, we have to run the app on different goroutine than testing's goroutine.
	if cfg.LogLevel != "" {
		log.SetLevel(cfg.LogLevel)
	}

	testnet, err := wavelet.NewTestNetwork()
	if err != nil {
		return nil, err
	}

	port, err := nextPort()
	if err != nil {
		return nil, err
	}

	apiPort, err := nextPort()
	if err != nil {
		return nil, err
	}

	args := []string{"wavelet", "--loglevel", "", "--port", port, "--api.port", apiPort}
	if cfg != nil && cfg.Wallet != "" {
		args = append(args, []string{"--wallet", cfg.Wallet}...)
	}

	// Bootstrap with the faucet
	args = append(args, testnet.Faucet().Addr())

	stdin := mockStdin(make(chan string))
	stdout := newMockStdout()

	w := &TestWavelet{
		Testnet: testnet,
		Port:    port,
		APIPort: apiPort,
		Stdin:   stdin,
		Stdout:  stdout,
	}

	w.StopWG.Add(1)
	go func() {
		defer w.StopWG.Done()
		Run(args, stdin, stdout, true)
	}()

	if err := waitForAPI(apiPort); err != nil {
		return nil, err
	}

	ls, err := w.GetLedgerStatus()
	if err != nil {
		return nil, err
	}

	w.PublicKey = ls.PublicKey

	return w, nil
}

func (w *TestWavelet) GetLedgerStatus() (*TestLedgerStatus, error) {
	ledger, err := getLedgerStatus(w.APIPort)
	if err != nil {
		return nil, err
	}

	return ledger, nil
}

func getLedgerStatus(apiPort string) (*TestLedgerStatus, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/ledger", apiPort), nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
	defer cancel()

	req = req.WithContext(ctx)

	client := &http.Client{
		Timeout: time.Second * 1,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("expecting GET /ledger to return 200, got %d instead", resp.StatusCode)
	}

	var ledger TestLedgerStatus
	if err := json.NewDecoder(resp.Body).Decode(&ledger); err != nil {
		return nil, err
	}

	return &ledger, nil
}

func (w *TestWavelet) WaitForTransaction(id string) (*TestTransaction, error) {
	var tx *TestTransaction

	err := waitFor(func() error {
		var err error
		tx, err = getTransaction(w.APIPort, id)
		return err
	})

	return tx, err
}

func (w *TestWavelet) GetTransaction(id string) (*TestTransaction, error) {
	return getTransaction(w.APIPort, id)
}

func getTransaction(apiPort string, id string) (*TestTransaction, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/tx/%s", apiPort, id), nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
	defer cancel()

	req = req.WithContext(ctx)

	client := &http.Client{
		Timeout: time.Second * 1,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("expecting GET /tx/%s to return 200, got %d instead", id, resp.StatusCode)
	}

	var tx TestTransaction
	if err := json.NewDecoder(resp.Body).Decode(&tx); err != nil {
		return nil, err
	}

	return &tx, nil
}

func (w *TestWavelet) WaitForConsensus(nodesNum int) error {
	_, err := w.Stdout.Search("Finalized block", nodesNum)
	return err
}

func asAccountID(s string) (wavelet.AccountID, error) {
	var accountID wavelet.AccountID

	key, err := hex.DecodeString(s)
	if err != nil {
		return accountID, err
	}

	copy(accountID[:], key)

	return accountID, nil
}
