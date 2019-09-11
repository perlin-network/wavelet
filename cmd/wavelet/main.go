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
	"io/ioutil"
	"os"
	"sort"
	"time"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"gopkg.in/urfave/cli.v1"
	"gopkg.in/urfave/cli.v1/altsrc"

	_ "net/http/pprof"
)

type Config struct {
	ServerAddr string // empty == start new server
}

func main() {
	log.SetWriter(
		log.LoggerWavelet,
		log.NewConsoleWriter(nil, log.FilterFor(
			log.ModuleNode,
			log.ModuleNetwork,
			log.ModuleSync,
			log.ModuleConsensus,
			log.ModuleContract,
		)),
	)

	logger := log.Node()

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
			Name:   "host",
			Value:  "127.0.0.1",
			Usage:  "Listen for peers on host address.",
			EnvVar: "WAVELET_NODE_HOST",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:   "port",
			Value:  3000,
			Usage:  "Listen for peers on port.",
			EnvVar: "WAVELET_NODE_PORT",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:   "api.port",
			Value:  0,
			Usage:  "Host a local HTTP API at port.",
			EnvVar: "WAVELET_API_PORT",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "wallet",
			Value:  "config/wallet.txt",
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
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.query_timeout",
			Value: int(sys.QueryTimeout.Seconds()),
			Usage: "Timeout in seconds for querying a transaction to K peers.",
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.max_depth_diff",
			Value: sys.MaxDepthDiff,
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
			Value:  sys.SnowballK,
			Usage:  "Snowball consensus protocol parameter k",
			EnvVar: "WAVELET_SNOWBALL_K",
		}),
		altsrc.NewFloat64Flag(cli.Float64Flag{
			Name:   "sys.snowball.alpha",
			Value:  sys.SnowballAlpha,
			Usage:  "Snowball consensus protocol parameter alpha",
			EnvVar: "WAVELET_SNOWBALL_ALPHA",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:   "sys.snowball.beta",
			Value:  sys.SnowballBeta,
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
		c.String("config")
		config := &wctl.Config{
			Host:     c.String("host"),
			Port:     c.Uint("port"),
			Wallet:   c.String("wallet"),
			APIPort:  c.Uint("api.port"),
			Peers:    c.Args(),
			Database: c.String("db"),
		}

		if genesis := c.String("genesis"); len(genesis) > 0 {
			config.Genesis = &genesis
		}

		// set the the sys variables
		sys.SnowballK = c.Int("sys.snowball.k")
		sys.SnowballAlpha = c.Float64("sys.snowball.alpha")
		sys.SnowballBeta = c.Int("sys.snowball.beta")
		sys.QueryTimeout = time.Duration(c.Int("sys.query_timeout")) * time.Second
		sys.MaxDepthDiff = c.Uint64("sys.max_depth_diff")
		sys.MinDifficulty = byte(c.Int("sys.difficulty.min"))
		sys.DifficultyScaleFactor = c.Float64("sys.difficulty.scale")
		sys.TransactionFeeAmount = c.Uint64("sys.transaction_fee_amount")
		sys.MinimumStake = c.Uint64("sys.min_stake")

		start(config)

		return nil
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(os.Args); err != nil {
		logger.Fatal().Err(err).
			Msg("Failed to parse configuration/command-line arguments.")
	}
}

func start(cfg *Config) {

	c, err := wctl.NewClient(wctl.Config{
		APIHost:    "localhost",
		APIPort:    uint16(cfg.APIPort),
		PrivateKey: keys.PrivateKey(),
	})

	if err != nil {
		logger.Fatal().Err(err).
			Uint("port", cfg.APIPort).
			Msg("Failed to connect to API")
	}

	// Set CLI callbacks, mainly loggers
	if err := setEvents(c); err != nil {
		logger.Fatal().Err(err).
			Msg("Failed to start websockets to the server")
	}

	shell, err := NewCLI(c)
	if err != nil {
		logger.Fatal().Err(err).
			Msg("Failed to spawn the CLI")
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
