package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"github.com/phayes/freeport"
	"github.com/stretchr/testify/assert"
)

var defaultWallet = "87a6813c3b4cf534b6ae82db9b1409fa7dbd5c13dba5858970b56084c4a930eb400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405"

func TestMain(t *testing.T) {
	port := nextPort(t)
	apiPort := nextPort(t)

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort},
		nopStdin(), ioutil.Discard)
	waitForAPI(t, apiPort)

	ledger := getLedgerStatusT(t, apiPort)
	assert.EqualValues(t, "127.0.0.1:"+port, ledger.Address)
	assert.NotEqual(t, defaultWallet[64:], ledger.PublicKey) // A random wallet should be generated
	assert.Nil(t, ledger.Peers)
}

func TestMain_WithWalletString(t *testing.T) {
	wallet := "b27b880e6e44e3b127186a08bc5698316e8dd99157cec56211560b62141f0851c72096021609681eb8cab244752945b2008e1b51d8bc2208b2b562f35485d5cc"

	port := nextPort(t)
	apiPort := nextPort(t)

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", wallet},
		nopStdin(), ioutil.Discard)
	waitForAPI(t, apiPort)

	ledger := getLedgerStatusT(t, apiPort)
	assert.EqualValues(t, wallet[64:], ledger.PublicKey)
}

func TestMain_WithWalletFile(t *testing.T) {
	wallet := "d6acf5caca96e9da2088bc3a051ed938749145c3f3f7b5bd81aefeb46c12f9e901e4d5b4f51ef76a9a9cc511af910964943404172347b6a01bcfe65d768c9354"

	// Write wallet to a temporary file
	dir, err := ioutil.TempDir("", "wavelet")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	walletPath := filepath.Join(dir, "wallet.txt")
	if err := ioutil.WriteFile(walletPath, []byte(wallet), 0666); err != nil {
		t.Fatal(err)
	}

	port := nextPort(t)
	apiPort := nextPort(t)

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", walletPath},
		nopStdin(), ioutil.Discard)
	waitForAPI(t, apiPort)

	ledger := getLedgerStatusT(t, apiPort)
	assert.EqualValues(t, wallet[64:], ledger.PublicKey)
}

func TestMain_WithInvalidWallet(t *testing.T) {
	wallet := "foobar"

	port := nextPort(t)
	apiPort := nextPort(t)

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", wallet},
		nopStdin(), ioutil.Discard)
	waitForAPI(t, apiPort)

	ledger := getLedgerStatusT(t, apiPort)
	assert.EqualValues(t, "127.0.0.1:"+port, ledger.Address)
	assert.NotEqual(t, defaultWallet[64:], ledger.PublicKey) // A random wallet should be generated
	assert.Nil(t, ledger.Peers)
}

func TestMain_Status(t *testing.T) {
	port := nextPort(t)
	apiPort := nextPort(t)

	stdin := make(chan string)
	stdout := newMockStdout()

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", defaultWallet},
		mockStdin(stdin), stdout)

	waitForAPI(t, apiPort)

	stdin <- "status"
	stdout.Search(t, "Here is the current status of your node")
}

func TestMain_Pay(t *testing.T) {
	keys, err := skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		t.Fatal(err)
	}

	port := nextPort(t)
	apiPort := nextPort(t)

	stdin := make(chan string)
	stdout := newMockStdout()

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", defaultWallet},
		mockStdin(stdin), stdout)

	waitForAPI(t, apiPort)

	recipient := keys.PublicKey()
	stdin <- fmt.Sprintf("p %s 99999", hex.EncodeToString(recipient[:]))

	txID := extractTxID(t, stdout.Search(t, "Success! Your payment transaction ID:"))

	var tx *TestTransaction
	waitFor(t, func() error {
		tx, err = getTransaction(apiPort, txID)
		return err
	})

	assert.EqualValues(t, txID, tx.ID)
	assert.EqualValues(t, defaultWallet[64:], tx.Sender)
	assert.EqualValues(t, defaultWallet[64:], tx.Creator)
}

func TestMain_Find(t *testing.T) {
	keys, err := skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		t.Fatal(err)
	}

	port := nextPort(t)
	apiPort := nextPort(t)

	stdin := make(chan string)
	stdout := newMockStdout()

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", defaultWallet},
		mockStdin(stdin), stdout)

	waitForAPI(t, apiPort)

	recipient := keys.PublicKey()
	stdin <- fmt.Sprintf("p %s 99999", hex.EncodeToString(recipient[:]))

	txID := extractTxID(t, stdout.Search(t, "Success! Your payment transaction ID:"))

	// Wait for the transction to be available
	waitFor(t, func() error {
		_, err := getTransaction(apiPort, txID)
		return err
	})

	stdin <- fmt.Sprintf("find %s", txID)
	stdout.Search(t, fmt.Sprintf("Transaction: %s", txID))
}

func TestMain_PlaceStake(t *testing.T) {
	port := nextPort(t)
	apiPort := nextPort(t)

	stdin := make(chan string)
	stdout := newMockStdout()

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", defaultWallet},
		mockStdin(stdin), stdout)

	waitForAPI(t, apiPort)

	stdin <- "ps 1000"

	txID := extractTxID(t, stdout.Search(t, "Success! Your stake placement transaction ID:"))
	tx := getTransactionT(t, apiPort, txID)

	assert.EqualValues(t, txID, tx.ID)
	assert.EqualValues(t, defaultWallet[64:], tx.Sender)
	assert.EqualValues(t, defaultWallet[64:], tx.Creator)
}

func TestMain_WithdrawStake(t *testing.T) {
	port := nextPort(t)
	apiPort := nextPort(t)

	stdin := make(chan string)
	stdout := newMockStdout()

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", defaultWallet},
		mockStdin(stdin), stdout)

	waitForAPI(t, apiPort)

	stdin <- "ws 1000"

	txID := extractTxID(t, stdout.Search(t, "Success! Your stake withdrawal transaction ID:"))
	tx := getTransactionT(t, apiPort, txID)

	assert.EqualValues(t, txID, tx.ID)
	assert.EqualValues(t, defaultWallet[64:], tx.Sender)
	assert.EqualValues(t, defaultWallet[64:], tx.Creator)
}

func TestMain_WithdrawReward(t *testing.T) {
	port := nextPort(t)
	apiPort := nextPort(t)

	stdin := make(chan string)
	stdout := newMockStdout()

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", defaultWallet},
		mockStdin(stdin), stdout)

	waitForAPI(t, apiPort)

	stdin <- "wr 1000"

	txID := extractTxID(t, stdout.Search(t, "Success! Your reward withdrawal transaction ID:"))
	tx := getTransactionT(t, apiPort, txID)

	assert.EqualValues(t, txID, tx.ID)
	assert.EqualValues(t, defaultWallet[64:], tx.Sender)
	assert.EqualValues(t, defaultWallet[64:], tx.Creator)
}

func nextPort(t *testing.T) string {
	port, err := freeport.GetFreePort()
	if err != nil {
		t.Fatal(err)
	}
	return strconv.Itoa(port)
}

func waitForAPI(t *testing.T, apiPort string) {
	timeout := time.NewTimer(time.Second * 5)
	tick := time.NewTicker(time.Second * 1)

	for {
		select {
		case <-timeout.C:
			t.Fatal("timed out waiting for API")

		case <-tick.C:
			if _, err := getLedgerStatus(apiPort); err == nil {
				return
			}
		}
	}
}

type TestLedgerStatus struct {
	PublicKey string   `json:"public_key"`
	Address   string   `json:"address"`
	Peers     []string `json:"peers"`
}

type TestTransaction struct {
	ID      string `json:"id"`
	Sender  string `json:"sender"`
	Creator string `json:"creator"`
}

func getLedgerStatusT(t *testing.T, apiPort string) *TestLedgerStatus {
	ledger, err := getLedgerStatus(apiPort)
	if err != nil {
		t.Fatal(err)
	}
	return ledger
}

func getLedgerStatus(apiPort string) (*TestLedgerStatus, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/ledger", apiPort), nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
	defer cancel()

	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("expecting GET /ledger to return 200, got %d instead", resp.StatusCode)
	}

	var ledger TestLedgerStatus
	if err := json.NewDecoder(resp.Body).Decode(&ledger); err != nil {
		return nil, err
	}

	return &ledger, nil
}

func getTransactionT(t *testing.T, apiPort string, id string) *TestTransaction {
	tx, err := getTransaction(apiPort, id)
	if err != nil {
		t.Fatal(err)
	}

	return tx
}

func getTransaction(apiPort string, id string) (*TestTransaction, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/tx/%s", apiPort, id), nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
	defer cancel()

	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("expecting GET /tx/%s to return 200, got %d instead", id, resp.StatusCode)
	}

	var tx TestTransaction
	if err := json.NewDecoder(resp.Body).Decode(&tx); err != nil {
		return nil, err
	}

	return &tx, nil
}

func nopStdin() io.ReadCloser {
	return ioutil.NopCloser(strings.NewReader(""))
}

type mockStdin chan string

func (s mockStdin) Read(dst []byte) (n int, err error) {
	line := <-s
	if line == "" {
		return 0, io.EOF
	}

	copy(dst, []byte(line+"\n"))
	return len(line) + 1, nil
}

func (s mockStdin) Close() error {
	close(s)
	return nil
}

type mockStdout struct {
	Lines chan string
	buf   []byte
	bi    int
}

func newMockStdout() *mockStdout {
	return &mockStdout{
		Lines: make(chan string, 256),
		buf:   make([]byte, 2048),
		bi:    0,
	}
}

func (s *mockStdout) Write(p []byte) (n int, err error) {
	ni := bytes.Index(p, []byte{'\n'})
	if ni < 0 {
		copy(s.buf[s.bi:], p)
		s.bi += len(p)
		return len(p), nil
	}

	for ni >= 0 {
		copy(s.buf[s.bi:], p[:ni])
		s.bi += ni

		s.Lines <- string(s.buf[:s.bi])

		s.bi = 0
		p = p[ni+1:]
		ni = bytes.Index(p, []byte{'\n'})
	}

	return len(p), nil
}

func (s *mockStdout) Search(t *testing.T, sub string) string {
	timeout := time.NewTimer(time.Second * 5)
	for {
		select {
		case line := <-s.Lines:
			if strings.Contains(line, sub) {
				return line
			}

		case <-timeout.C:
			t.Fatal("timed out searching for string in stdout")
		}
	}
}

func extractTxID(t *testing.T, s string) string {
	if len(s) < 64 {
		t.Fatal("output does not contain tx id")
	}

	txID := s[len(s)-64:]
	if _, err := hex.DecodeString(txID); err != nil {
		t.Fatal(err)
	}

	return txID
}

func waitFor(t *testing.T, fn func() error) {
	timeout := time.NewTimer(time.Second * 10)
	ticker := time.NewTicker(time.Second * 1)

	for {
		select {
		case <-timeout.C:
			t.Fatal("timed out waiting")

		case <-ticker.C:
			if err := fn(); err == nil {
				return
			}
		}
	}
}
