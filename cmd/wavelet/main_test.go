package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/phayes/freeport"
	"github.com/stretchr/testify/assert"
)

var defaultWallet = "87a6813c3b4cf534b6ae82db9b1409fa7dbd5c13dba5858970b56084c4a930eb400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405"

func TestMain(t *testing.T) {
	port := nextPort(t)
	apiPort := nextPort(t)

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort})
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

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", wallet})
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

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", walletPath})
	waitForAPI(t, apiPort)

	ledger := getLedgerStatusT(t, apiPort)
	assert.EqualValues(t, wallet[64:], ledger.PublicKey)
}

func TestMain_WithInvalidWallet(t *testing.T) {
	wallet := "foobar"

	port := nextPort(t)
	apiPort := nextPort(t)

	go Run([]string{"wavelet", "--port", port, "--api.port", apiPort, "--wallet", wallet})
	waitForAPI(t, apiPort)

	ledger := getLedgerStatusT(t, apiPort)
	assert.EqualValues(t, "127.0.0.1:"+port, ledger.Address)
	assert.NotEqual(t, defaultWallet[64:], ledger.PublicKey) // A random wallet should be generated
	assert.Nil(t, ledger.Peers)
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
