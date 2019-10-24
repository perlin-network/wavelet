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
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/cmd/wavelet/node"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
	"gopkg.in/urfave/cli.v1/altsrc"

	_ "net/http/pprof"
	"net/url"
)

// default false, used for testing
var disableGC bool

var logger = log.Node()

type Config struct {
	ServerAddr string // empty == start new server
	UpdateURL  string
}

func main() {
	// switchToUpdatedVersion()
	wavelet.SetGenesisByNetwork(sys.VersionMeta)

	Run(os.Args, os.Stdin, os.Stdout)
}

func Run(args []string, stdin io.ReadCloser, stdout io.Writer) {
	log.SetWriter(log.LoggerWavelet, log.NewConsoleWriter(
		stdout, log.FilterFor(log.ModuleNode)))

	app := cli.NewApp()

	app.Name = "wavelet"
	app.Author = "Perlin"
	app.Email = "support@perlin.net"
	app.Version = sys.Version
	app.Usage = "a bleeding fast ledger with a powerful compute layer"

	app.Flags = []cli.Flag{
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "server",
			Value:  "",
			Usage:  "Connect to a Wavelet server instead of hosting a new one if not blank.",
			EnvVar: "WAVELET_SERVER",
		}),
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
			Value:  9000,
			Usage:  "Host a local HTTP API at port.",
			EnvVar: "WAVELET_API_PORT",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "api.host",
			Usage:  "Host for the API HTTPS node.",
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
			Usage:  "Genesis JSON file contents representing initial fields of some set of accounts at block 0.",
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
			Name:  "sys.transaction_fee_amount",
			Value: sys.DefaultTransactionFee,
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
		altsrc.NewIntFlag(cli.IntFlag{
			Name:   "sys.snowball.beta",
			Value:  conf.GetSnowballBeta(),
			Usage:  "Snowball consensus protocol parameter beta",
			EnvVar: "WAVELET_SNOWBALL_BETA",
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
		return start(c, stdin, stdout)
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(args); err != nil {
		logger := log.Node()
		logger.Fatal().Err(err).
			Msg("Failed to parse configuration/command-line arguments.")
	}
}

func start(c *cli.Context, stdin io.ReadCloser, stdout io.Writer) error {
	config := Config{
		ServerAddr: c.String("server"),
		UpdateURL:  c.String("update-url"),
	}

	// Set the log level
	log.SetLevel(c.String("loglevel"))

	// Start the background updater
	// go periodicUpdateRoutine(c.String("update-url"))

	w, err := wallet(c.String("wallet"))
	if err != nil {
		return err
	}

	// Grab the secret
	secret := c.String("api.secret")
	if secret == "" {
		// If the secret is empty, derive the secret from the base64-hashed
		// sha224-hashed private key.
		h, err := hex.DecodeString(secret)
		if err != nil {
			return errors.Wrap(err, "Can't decode wallet")
		}

		sha := sha512.Sum512_224(h)
		secret = base64.StdEncoding.EncodeToString(sha[:])
	}

	conf.Update(
		conf.WithSnowballK(c.Int("sys.snowball.k")),
		conf.WithSnowballBeta(c.Int("sys.snowball.beta")),
		conf.WithQueryTimeout(c.Duration("sys.query_timeout")),
		conf.WithSecret(secret),
	)

	// set the the sys variables
	sys.DefaultTransactionFee = c.Uint64("sys.transaction_fee_amount")
	sys.MinimumStake = c.Uint64("sys.min_stake")

	var wctlCfg wctl.Config
	wctlCfg.APISecret = conf.GetSecret()

	if config.ServerAddr == "" {
		srvCfg := node.Config{
			NAT:         c.Bool("nat"),
			Host:        c.String("host"),
			Port:        c.Uint("port"),
			Wallet:      w,
			APIPort:     c.Uint("api.port"),
			Peers:       c.Args(),
			Database:    c.String("db"),
			MaxMemoryMB: c.Uint64("memory.max"),
			// HTTPS
			APIHost:       c.String("api.host"),
			APICertsCache: c.String("api.certs"),
			// Debugging only
			NoGC: disableGC,
		}

		if genesis := c.String("genesis"); len(genesis) > 0 {
			srvCfg.Genesis = &genesis
		}

		srv, err := node.New(&srvCfg)
		if err != nil {
			return err
		}

		// Start the server
		srv.Start()
		defer srv.Close()

		wctlCfg.Server = srv
		wctlCfg.APIPort = uint16(c.Uint("api.port"))
		wctlCfg.PrivateKey = srv.Keys.PrivateKey()

		wctlCfg.APIHost = c.String("api.host")
		if wctlCfg.APIHost == "" {
			wctlCfg.APIHost = "127.0.0.1"
		} else {
			wctlCfg.UseHTTPS = true
		}
	} else {
		u, err := url.Parse(config.ServerAddr)
		if err != nil {
			return fmt.Errorf("Invalid address: %v", err)
		}

		port, _ := strconv.ParseUint(u.Port(), 10, 16)

		wctlCfg.APIPort = uint16(port)
		wctlCfg.APIHost = u.Host
		//PrivateKey = nil // TODO?
		wctlCfg.UseHTTPS = u.Scheme == "https"
	}

	client, err := wctl.NewClient(wctlCfg)
	if err != nil {
		return err
	}

	defer client.Close()

	shell, err := NewCLI(client, CLIWithStdin(stdin), CLIWithStdout(stdout))
	if err != nil {
		return fmt.Errorf("Failed to spawn the CLI: %v", err)
	}

	shell.Start()
	return nil
}

// returns hex-encoded
func wallet(wallet string) (string, error) {
	var keys *skademlia.Keypair

	logger := log.Node()

	privateKeyBuf, err := ioutil.ReadFile(wallet)

	// File exists
	if err == nil {
		// If the file content is the actual private key
		if hex.DecodedLen(len(privateKeyBuf)) == edwards25519.SizePrivateKey {
			return string(privateKeyBuf), nil
		}
	}

	if os.IsNotExist(err) {
		// If a private key is specified, simply use the provided private key instead.
		if len(wallet) == hex.EncodedLen(edwards25519.SizePrivateKey) {
			return wallet, nil
		}
	}

	keys, err = skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		return "", errors.New("failed to generate a new wallet")
	}

	privateKey := keys.PrivateKey()
	publicKey := keys.PublicKey()

	logger.Info().
		Hex("privateKey", privateKey[:]).
		Hex("publicKey", publicKey[:]).
		Msg("Existing wallet not found: generated a new one.")

	return hex.EncodeToString(privateKey[:]), nil
}
