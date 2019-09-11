package server

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/handshake"
	"github.com/perlin-network/noise/nat"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/internal/snappy"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type Config struct {
	NAT      bool
	Host     string
	Port     uint
	Wallet   string // hex encoded
	Genesis  *string
	APIPort  uint
	Peers    []string
	Database string
}

var DefaultConfig = Config{
	Host:     "127.0.0.1",
	Port:     3000,
	Wallet:   "87a6813c3b4cf534b6ae82db9b1409fa7dbd5c13dba5858970b56084c4a930eb400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
	Genesis:  nil,
	APIPort:  9000,
	Peers:    []string{},
	Database: "",
}

type Wavelet struct {
	SKademlia *skademlia.Client
	Keypair   *skademlia.Keypair
	Ledger    *wavelet.Ledger
	Keys      *skademlia.Keypair

	config   *Config
	logger   zerolog.Logger
	listener net.Listener
}

func New(cfg *Config) (*Wavelet, error) {
	if cfg == nil {
		cfg = &DefaultConfig
	}

	w := Wavelet{
		config: cfg,
	}

	// Make a logger
	logger := log.Node()
	w.logger = logger

	// TODO(diamond): change all panics to useful logger.Fatals

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return nil, fmt.Errorf("Failed to open port %d: %v", cfg.Port, err)
	}

	w.listener = listener

	addr := net.JoinHostPort(
		cfg.Host, strconv.Itoa(listener.Addr().(*net.TCPAddr).Port),
	)

	if cfg.NAT {
		if len(cfg.Peers) > 1 {
			resolver := nat.NewPMP()

			if err := resolver.AddMapping("tcp",
				uint16(listener.Addr().(*net.TCPAddr).Port),
				uint16(listener.Addr().(*net.TCPAddr).Port),
				30*time.Minute,
			); err != nil {
				return nil, fmt.Errorf("Failed to initialize NAT: %v", err)
			}
		}

		resp, err := http.Get("http://myexternalip.com/raw")
		if err != nil {
			return nil, fmt.Errorf("Failed to get external IP: %v", err)
		}

		defer resp.Body.Close()

		ip, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Failed to get external IP: %v", err)
		}

		addr = net.JoinHostPort(
			string(ip), strconv.Itoa(listener.Addr().(*net.TCPAddr).Port),
		)
	}

	logger.Info().Str("addr", addr).
		Msg("Listening for peers.")

	// Load keys
	var privateKey edwards25519.PrivateKey

	i, err := hex.Decode(privateKey[:], []byte(cfg.Wallet))
	if err != nil {
		return nil, fmt.Errorf("Failed to decode hex wallet %s", cfg.Wallet)
	}

	if i != edwards25519.SizePrivateKey {
		return nil, fmt.Errorf("Wallet is not of the right length (%d not %d)",
			i, edwards25519.SizePrivateKey)
	}

	keys, err := skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		return nil, fmt.Errorf("The wallet specified is invalid: %v", err)
	}

	w.Keypair = keys

	client := skademlia.NewClient(
		addr, keys,
		skademlia.WithC1(sys.SKademliaC1),
		skademlia.WithC2(sys.SKademliaC2),
		skademlia.WithDialOptions(grpc.WithDefaultCallOptions(
			grpc.UseCompressor(snappy.Name))),
	)

	client.SetCredentials(noise.NewCredentials(
		addr, handshake.NewECDH(), cipher.NewAEAD(), client.Protocol(),
	))

	w.SKademlia = client

	kv, err := store.NewLevelDB(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf(
			"Failed to create/open database located at %s", cfg.Database)
	}

	ledger := wavelet.NewLedger(kv, client, wavelet.WithGenesis(cfg.Genesis))
	w.Ledger = ledger

	return &w, nil
}

func (w *Wavelet) Start() {
	go func() {
		server := w.SKademlia.Listen()
		wavelet.RegisterWaveletServer(server, w.Ledger.Protocol())
		if err := server.Serve(w.listener); err != nil {
			w.logger.Fatal().Err(err).
				Msg("S/Kademlia failed to listen")
		}
	}()

	for _, addr := range w.config.Peers {
		if _, err := w.SKademlia.Dial(addr); err != nil {
			w.logger.Warn().Err(err).
				Str("addr", addr).
				Msg("Error dialing")
		}
	}

	if peers := w.SKademlia.Bootstrap(); len(peers) > 0 {
		var ids []string

		for _, id := range peers {
			ids = append(ids, id.String())
		}

		w.logger.Info().Msgf("Bootstrapped with peers: %+v", ids)
	}

	if w.config.APIPort == 0 {
		w.config.APIPort = 9000
	}

	go api.New().StartHTTP(
		int(w.config.Port), w.SKademlia, w.Ledger, w.Keypair)
}
