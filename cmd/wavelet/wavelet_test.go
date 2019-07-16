package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/phayes/freeport"
)

type TestWavelet struct {
	cmd    *exec.Cmd
	pty    *os.File
	config *Config
}

type TestLedgerStatus struct {
	PublicKey string `json:"public_key"`
	Address   string `json:"address"`
}

// New starts a Wavelet process.
func New(t *testing.T, cb func(cfg *Config)) *TestWavelet {
	path, err := exec.LookPath("wavelet")
	if err != nil || path == "" {
		t.Fatal("wavelet not found on $PATH")
	}

	cfg := defaultTestConfig(t)
	if cb != nil {
		cb(cfg)
	}

	args := []string{}

	if cfg.Port > 0 {
		args = append(args, []string{"--port", fmt.Sprintf("%d", cfg.Port)}...)
	}
	if cfg.APIPort > 0 {
		args = append(args, []string{"--api.port", fmt.Sprintf("%d", cfg.APIPort)}...)
	}
	if cfg.Wallet != "" {
		args = append(args, []string{"--wallet", cfg.Wallet}...)
	}

	cmd := exec.Command("wavelet", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Use pty to force the process to run in interactive mode.
	// Otherwise, the process will immediately exit because stdin
	// will immediately return io.EOF.
	pty, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}

	wavelet := &TestWavelet{cmd: cmd, pty: pty, config: cfg}

	// Make sure Wavelet starts successfully by checking the API
	if err := wavelet.waitForAPI(t); err != nil {
		defer wavelet.Stop()
		t.Fatal(err)
	}

	return wavelet
}

func defaultTestConfig(t *testing.T) *Config {
	return &Config{
		Port:    nextPort(t),
		APIPort: nextPort(t),
		Wallet:  "",
	}
}

func nextPort(t *testing.T) uint {
	port, err := freeport.GetFreePort()
	if err != nil {
		t.Fatal(err)
	}

	return uint(port)
}

func (w *TestWavelet) waitForAPI(t *testing.T) error {
	timeout := time.After(3 * time.Second)
	tick := time.Tick(1 * time.Second)

	var lastErr error
	for {
		select {
		case <-timeout:
			return lastErr

		case <-tick:
			if _, err := w.getLedgerStatus(); err != nil {
				lastErr = err
				break
			}

			return nil
		}
	}
}

// Stop interrupts the Wavelet process and waits for it to shutdown.
func (w *TestWavelet) Stop() error {
	if w.cmd == nil {
		return nil
	}

	if w.cmd.Process != nil {
		if err := w.cmd.Process.Signal(os.Interrupt); err != nil {
			return err
		}
	}

	return w.cmd.Wait()
}

// GetLedgerStatus returns the ledger status.
func (w *TestWavelet) GetLedgerStatus(t *testing.T) *TestLedgerStatus {
	ledger, err := w.getLedgerStatus()
	if err != nil {
		t.Fatal(err)
	}

	return ledger
}

func (w *TestWavelet) getLedgerStatus() (*TestLedgerStatus, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/ledger", w.config.APIPort), nil)
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
