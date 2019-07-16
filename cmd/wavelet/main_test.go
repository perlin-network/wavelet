package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

var wallet1 = "87a6813c3b4cf534b6ae82db9b1409fa7dbd5c13dba5858970b56084c4a930eb400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405"
var wallet2 = "85e7450f7cf0d9cd1d1d7bf4169c2f364eea4ba833a7280e0f931a1d92fd92c2696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a"
var wallet3 = "5b9fcd2d6f8e34f4aa472e0c3099fefd25f0ceab9e908196b1dda63e55349d22f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467"

func TestWavelet_Default(t *testing.T) {
	wavelet := New(t, nil)
	defer wavelet.Stop()

	ledger := wavelet.GetLedgerStatus(t)
	// Make sure a random wallet is generated
	assert.NotEqual(t, wallet1, ledger.PublicKey)
	assert.NotEqual(t, wallet2, ledger.PublicKey)
	assert.NotEqual(t, wallet3, ledger.PublicKey)
}

func TestWavelet_InvalidWallet(t *testing.T) {
	wavelet := New(t, func(cfg *Config) {
		cfg.Wallet = "foobar"
	})
	defer wavelet.Stop()

	ledger := wavelet.GetLedgerStatus(t)
	// If specified wallet is not a valid private key,
	// a random wallet is also generated
	assert.NotEqual(t, wallet1, ledger.PublicKey)
	assert.NotEqual(t, wallet2, ledger.PublicKey)
	assert.NotEqual(t, wallet3, ledger.PublicKey)
}

func TestWavelet_WithWalletString(t *testing.T) {
	wavelet := New(t, func(cfg *Config) {
		cfg.Wallet = wallet2
	})
	defer wavelet.Stop()

	ledger := wavelet.GetLedgerStatus(t)
	assert.Equal(t, wallet2[64:], ledger.PublicKey)
}

func TestWavelet_WithWalletPath(t *testing.T) {
	// Write wallet3 to a temporary file
	dir, err := ioutil.TempDir("", "wavelet")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	walletPath := filepath.Join(dir, "wallet3.txt")
	if err := ioutil.WriteFile(walletPath, []byte(wallet3), 0666); err != nil {
		t.Fatal(err)
	}

	wavelet := New(t, func(cfg *Config) {
		cfg.Wallet = walletPath
	})
	defer wavelet.Stop()

	ledger := wavelet.GetLedgerStatus(t)
	assert.Equal(t, wallet3[64:], ledger.PublicKey)
}
