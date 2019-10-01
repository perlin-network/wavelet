// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/perlin-network/wavelet/conf"

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
	"google.golang.org/grpc"
	"gopkg.in/urfave/cli.v1"
	"gopkg.in/urfave/cli.v1/altsrc"

	_ "net/http/pprof"
)

type Config struct {
	NAT           bool
	Host          string
	UpdateURL     string
	Port          uint
	Wallet        string
	Genesis       *string
	LogLevel      string
	APIPort       uint
	APIHost       *string
	APICertsCache *string
	Peers         []string
	Database      string
	MaxMemoryMB   uint64

	// Only for testing
	WithoutGC bool
}

func main() {
	switchToUpdatedVersion()
	Run(os.Args, os.Stdin, os.Stdout, false)
}

func Run(args []string, stdin io.ReadCloser, stdout io.Writer, withoutGC bool) {
	log.SetWriter(
		log.LoggerWavelet,
		log.NewConsoleWriter(stdout, log.FilterFor(
			log.ModuleNode,
			log.ModuleNetwork,
			log.ModuleSync,
			log.ModuleConsensus,
			log.ModuleContract,
		)),
	)

	app := cli.NewApp()

	app.Name = "wavelet"
	app.Author = "Perlin"
	app.Email = "support@perlin.net"
	app.Version = sys.Version
	app.Usage = "a bleeding fast ledger with a powerful compute layer"

	app.Flags = []cli.Flag{
		altsrc.NewBoolFlag(cli.BoolFlag{
			Name:   "nat",
			Usage:  "Enable port forwarding: only required for personal PCs.",
			EnvVar: "WAVELET_NAT",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "update-url",
			Value:  "https://updates.perlin.net/wavelet",
			Usage:  "URL for updating Wavelet node.",
			EnvVar: "WAVELET_UPDATE_URL",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "host",
			Value:  "127.0.0.1",
			Usage:  "Listen for peers on host address.",
			EnvVar: "WAVELET_NODE_HOST",
		}),
		altsrc.NewUintFlag(cli.UintFlag{
			Name:   "port",
			Value:  3000,
			Usage:  "Listen for peers on port.",
			EnvVar: "WAVELET_NODE_PORT",
		}),
		altsrc.NewUintFlag(cli.UintFlag{
			Name:   "api.port",
			Value:  0,
			Usage:  "Host a local HTTP API at port.",
			EnvVar: "WAVELET_API_PORT",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "api.host",
			Usage:  "Host for the API HTTPS server.",
			EnvVar: "WAVELET_API_HOST",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "api.certs",
			Usage:  "Directory path to cache HTTPS certificates.",
			EnvVar: "WAVELET_CERTS_CACHE_DIR",
		}),
		cli.StringFlag{
			Name:   "api.secret",
			Value:  conf.GetSecret(),
			Usage:  "Shared secret to restrict access to some api",
			EnvVar: "WAVELET_API_SECRET",
		},
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "wallet",
			Usage:  "Path to file containing hex-encoded private key. If the path specified is invalid, or no file exists at the specified path, a random wallet will be generated. Optionally, a 128-length hex-encoded private key to a wallet may also be specified.",
			EnvVar: "WAVELET_WALLET",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "genesis",
			Usage:  "Genesis JSON file contents representing initial fields of some set of accounts at round 0.",
			EnvVar: "WAVELET_GENESIS",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "db",
			Usage:  "Directory path to the database. If empty, a temporary in-memory database will be used instead.",
			EnvVar: "WAVELET_DB_PATH",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "loglevel",
			Value:  "debug",
			Usage:  "Minimum log level to output. Possible values: debug, info, warn, error, fatal, panic.",
			EnvVar: "WAVELET_LOGLEVEL",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:   "memory.max",
			Value:  0,
			Usage:  "Maximum memory in MB allowed to be used by wavelet.",
			EnvVar: "WAVELET_MEMORY_MAX",
		}),
		altsrc.NewDurationFlag(cli.DurationFlag{
			Name:  "sys.query_timeout",
			Value: conf.GetQueryTimeout(),
			Usage: "Timeout in seconds for querying a transaction to K peers.",
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.max_depth_diff",
			Value: conf.GetMaxDepthDiff(),
			Usage: "Max graph depth difference to search for eligible transaction parents from for our node.",
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.transaction_fee_amount",
			Value: sys.TransactionFeeAmount,
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.min_stake",
			Value: sys.MinimumStake,
			Usage: "minimum stake to garner validator rewards and have importance in consensus",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:   "sys.snowball.k",
			Value:  conf.GetSnowballK(),
			Usage:  "Snowball consensus protocol parameter k",
			EnvVar: "WAVELET_SNOWBALL_K",
		}),
		altsrc.NewFloat64Flag(cli.Float64Flag{
			Name:   "sys.snowball.alpha",
			Value:  conf.GetSnowballAlpha(),
			Usage:  "Snowball consensus protocol parameter alpha",
			EnvVar: "WAVELET_SNOWBALL_ALPHA",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:   "sys.snowball.beta",
			Value:  conf.GetSnowballBeta(),
			Usage:  "Snowball consensus protocol parameter beta",
			EnvVar: "WAVELET_SNOWBALL_BETA",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.difficulty.min",
			Value: int(sys.MinDifficulty),
			Usage: "Minimum difficulty to define a critical transaction",
		}),
		altsrc.NewFloat64Flag(cli.Float64Flag{
			Name:  "sys.difficulty.scale",
			Value: sys.DifficultyScaleFactor,
			Usage: "Factor to scale a transactions confidence down by to compute the difficulty needed to define a critical transaction",
		}),
		cli.StringFlag{
			Name:  "config, c",
			Usage: "Path to TOML config file, will override the arguments.",
		},
	}

	// apply the toml before processing the flags
	app.Before = altsrc.InitInputSourceWithContext(
		app.Flags, func(c *cli.Context) (altsrc.InputSourceContext, error) {
			filePath := c.String("config")
			if len(filePath) > 0 {
				return altsrc.NewTomlSourceFromFile(filePath)
			}
			return &altsrc.MapInputSource{}, nil
		},
	)

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version:    %s\n", c.App.Version)
		fmt.Printf("Go Version: %s\n", sys.GoVersion)
		fmt.Printf("Git Commit: %s\n", sys.GitCommit)
		fmt.Printf("OS/Arch:    %s\n", sys.OSArch)
		fmt.Printf("Built:      %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	app.Action = func(c *cli.Context) error {
		config := &Config{
			Host:        c.String("host"),
			UpdateURL:   c.String("update-url"),
			Port:        c.Uint("port"),
			Wallet:      c.String("wallet"),
			APIPort:     c.Uint("api.port"),
			Peers:       c.Args(),
			Database:    c.String("db"),
			MaxMemoryMB: c.Uint64("memory.max"),
			WithoutGC:   withoutGC,
			LogLevel:    c.String("loglevel"),
		}

		if genesis := c.String("genesis"); len(genesis) > 0 {
			config.Genesis = &genesis
		}

		if apiHost := c.String("api.host"); len(apiHost) > 0 {
			config.APIHost = &apiHost

			if certsCache := c.String("api.certs"); len(certsCache) > 0 {
				config.APICertsCache = &certsCache
			} else {
				return errors.New("missing api.certs flag")
			}
		}

		conf.Update(
			conf.WithSnowballK(c.Int("sys.snowball.k")),
			conf.WithSnowballAlpha(c.Float64("sys.snowball.alpha")),
			conf.WithSnowballBeta(c.Int("sys.snowball.beta")),
			conf.WithQueryTimeout(c.Duration("sys.query_timeout")),
			conf.WithMaxDepthDiff(c.Uint64("sys.max_depth_diff")),
			conf.WithSecret(c.String("api.secret")),
		)

		// set the the sys variables
		sys.MinDifficulty = byte(c.Int("sys.difficulty.min"))
		sys.DifficultyScaleFactor = c.Float64("sys.difficulty.scale")
		sys.TransactionFeeAmount = c.Uint64("sys.transaction_fee_amount")
		sys.MinimumStake = c.Uint64("sys.min_stake")

		start(config, stdin, stdout)

		return nil
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(args); err != nil {
		logger := log.Node()
		logger.Fatal().Err(err).
			Msg("Failed to parse configuration/command-line arguments.")
	}
}

func start(cfg *Config, stdin io.ReadCloser, stdout io.Writer) {
	log.SetLevel(cfg.LogLevel)
	logger := log.Node()

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		panic(err)
	}

	go periodicUpdateRoutine(cfg.UpdateURL)

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))

	if cfg.NAT {
		if len(cfg.Peers) > 1 {
			resolver := nat.NewPMP()

			if err := resolver.AddMapping("tcp",
				uint16(listener.Addr().(*net.TCPAddr).Port),
				uint16(listener.Addr().(*net.TCPAddr).Port),
				30*time.Minute,
			); err != nil {
				panic(err)
			}
		}

		resp, err := http.Get("http://myexternalip.com/raw")
		if err != nil {
			panic(err)
		}

		ip, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}

		if err := resp.Body.Close(); err != nil {
			panic(err)
		}

		addr = net.JoinHostPort(string(ip), strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	}

	logger.Info().Str("addr", addr).Msg("Listening for peers.")

	keys, err := keys(cfg.Wallet)
	if err != nil {
		panic(err)
	}

	client := skademlia.NewClient(
		addr, keys,
		skademlia.WithC1(sys.SKademliaC1),
		skademlia.WithC2(sys.SKademliaC2),
		skademlia.WithDialOptions(grpc.WithDefaultCallOptions(
			grpc.UseCompressor(snappy.Name),
			grpc.MaxCallRecvMsgSize(9 * 1024 * 1024),
			grpc.MaxCallSendMsgSize(3 * 1024 * 1024),
		)),
	)

	client.SetCredentials(noise.NewCredentials(addr, handshake.NewECDH(), cipher.NewAEAD(), client.Protocol()))

	client.OnPeerJoin(func(conn *grpc.ClientConn, id *skademlia.ID) {
		publicKey := id.PublicKey()

		logger := log.Network("joined")
		logger.Info().
			Hex("public_key", publicKey[:]).
			Str("address", id.Address()).
			Msg("Peer has joined.")

	})

	client.OnPeerLeave(func(conn *grpc.ClientConn, id *skademlia.ID) {
		publicKey := id.PublicKey()

		logger := log.Network("left")
		logger.Info().
			Hex("public_key", publicKey[:]).
			Str("address", id.Address()).
			Msg("Peer has left.")
	})

	kv, err := store.NewBadger(cfg.Database)
	if err != nil {
		logger.Fatal().Err(err).Msgf("Failed to create/open database located at %q.", cfg.Database)
	}

	opts := []wavelet.Option{
		wavelet.WithGenesis(cfg.Genesis),
	}

	if cfg.WithoutGC {
		opts = append(opts, wavelet.WithoutGC())
	}
	if cfg.MaxMemoryMB > 0 {
		opts = append(opts, wavelet.WithMaxMemoryMB(cfg.MaxMemoryMB))
	}

	ledger := wavelet.NewLedger(kv, client, opts...)

	go func() {
		server := client.Listen()

		wavelet.RegisterWaveletServer(server, ledger.Protocol())

		if err := server.Serve(listener); err != nil {
			panic(err)
		}
	}()

	for _, addr := range cfg.Peers {
		if _, err := client.Dial(addr, skademlia.WithTimeout(10*time.Second)); err != nil {
			fmt.Printf("Error dialing %s: %v\n", addr, err)
		}
	}

	if peers := client.Bootstrap(); len(peers) > 0 {
		var ids []string

		for _, id := range peers {
			ids = append(ids, id.String())
		}

		logger.Info().Msgf("Bootstrapped with peers: %+v", ids)
	}

	if cfg.APIHost != nil {
		go api.New().StartHTTPS(int(cfg.APIPort), client, ledger, keys, kv, *cfg.APIHost, *cfg.APICertsCache)
	} else {
		if cfg.APIPort > 0 {
			go api.New().StartHTTP(int(cfg.APIPort), client, ledger, keys, kv)

		}
	}

	shell, err := NewCLI(client, ledger, keys, stdin, stdout, kv)
	if err != nil {
		panic(err)
	}

	shell.Start()
}

func keys(wallet string) (*skademlia.Keypair, error) {
	var keys *skademlia.Keypair

	logger := log.Node()

	privateKeyBuf, err := ioutil.ReadFile(wallet)

	if err == nil {
		var privateKey edwards25519.PrivateKey

		n, err := hex.Decode(privateKey[:], privateKeyBuf)
		if err != nil {
			return nil, fmt.Errorf("failed to decode your private key from %q", wallet)
		}

		if n != edwards25519.SizePrivateKey {
			return nil, fmt.Errorf("private key located in %q is not of the right length", wallet)
		}

		keys, err = skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
		if err != nil {
			return nil, fmt.Errorf("the private key specified in %q is invalid", wallet)
		}

		publicKey := keys.PublicKey()

		logger.Info().
			Hex("privateKey", privateKey[:]).
			Hex("publicKey", publicKey[:]).
			Msg("Wallet loaded.")

		return keys, nil
	}

	if os.IsNotExist(err) {
		// If a private key is specified instead of a path to a wallet, then simply use the provided private key instead.
		if len(wallet) == hex.EncodedLen(edwards25519.SizePrivateKey) {
			var privateKey edwards25519.PrivateKey

			n, err := hex.Decode(privateKey[:], []byte(wallet))
			if err != nil {
				return nil, fmt.Errorf("failed to decode the private key specified: %s", wallet)
			}

			if n != edwards25519.SizePrivateKey {
				return nil, fmt.Errorf("private key %s is not of the right length", wallet)
			}

			keys, err = skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
			if err != nil {
				return nil, fmt.Errorf("the private key specified is invalid: %s", wallet)
			}

			publicKey := keys.PublicKey()

			logger.Info().
				Hex("privateKey", privateKey[:]).
				Hex("publicKey", publicKey[:]).
				Msg("A private key was provided instead of a wallet file.")

			return keys, nil
		}

		keys, err = skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
		if err != nil {
			return nil, errors.New("failed to generate a new wallet")
		}

		privateKey := keys.PrivateKey()
		publicKey := keys.PublicKey()

		logger.Info().
			Hex("privateKey", privateKey[:]).
			Hex("publicKey", publicKey[:]).
			Msg("Existing wallet not found: generated a new one.")

		return keys, nil
	}

	return keys, err
}
