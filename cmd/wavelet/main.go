package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher/aead"
	"github.com/perlin-network/noise/handshake/ecdh"
	"github.com/perlin-network/noise/identity"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/sys"
	"github.com/rs/zerolog"
	"gopkg.in/urfave/cli.v1"
	"gopkg.in/urfave/cli.v1/altsrc"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultC2 = 16
	DefaultC1 = 16
)

type Config struct {
	Host    string
	Port    uint
	Wallet  string
	APIPort uint
}

func main() {
	log.Register(log.NewConsoleWriter(log.FilterFor("node", "consensus", "sync")))
	logger := log.Node()

	app := cli.NewApp()

	app.Name = "wavelet"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = sys.Version
	app.Usage = "a bleeding fast ledger with a powerful compute layer"

	app.Flags = []cli.Flag{
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "host",
			Value: "127.0.0.1",
			Usage: "Listen for peers on host address.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "port",
			Value: 3000,
			Usage: "Listen for peers on port.",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "wallet",
			Value: "config/wallet.txt",
			Usage: "path to file containing hex-encoded private key. If empty, a random wallet will be generated.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "api.port",
			Value: 0,
			Usage: "Host a local HTTP API at port.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.query_timeout",
			Value: 10,
			Usage: "Timeout in seconds for querying a transaction to K peers.",
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.max_eligible_parents_depth_diff",
			Value: 5,
			Usage: "Max graph depth difference to search for eligible transaction parents from for our node.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.median_timestamp_num_ancestors",
			Value: 5,
			Usage: "Number of ancestors to derive a median timestamp from.",
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.validator_reward_amount",
			Value: 2,
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.expected_consensus_time",
			Value: 1000,
			Usage: "In milliseconds.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.critical_timestamp_average_window_size",
			Value: 3,
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.min_stake",
			Value: 100,
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.snowball.k",
			Value: 1,
			Usage: "Snowball consensus protocol parameter k",
		}),
		altsrc.NewFloat64Flag(cli.Float64Flag{
			Name:  "sys.snowball.alpha",
			Value: 0.8,
			Usage: "Snowball consensus protocol parameter alpha",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.snowball.beta",
			Value: 10,
			Usage: "Snowball consensus protocol parameter beta",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.difficulty.min",
			Value: 5,
			Usage: "Maximum difficulty to define a critical transaction",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.difficulty.max",
			Value: 16,
			Usage: "Minimum difficulty to define a critical transaction",
		}),
		cli.StringFlag{
			Name:  "config, c",
			Usage: "Path to HCL config file, will override other arguments.",
		},
	}

	// apply the hcl before processing the flags
	app.Before = altsrc.InitInputSourceWithContext(app.Flags, func(c *cli.Context) (altsrc.InputSourceContext, error) {
		filePath := c.String("config")
		if len(filePath) > 0 {
			return altsrc.NewTomlSourceFromFile(filePath)
		}
		return &altsrc.MapInputSource{}, nil
	})

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version:    %s\n", c.App.Version)
		fmt.Printf("Go Version: %s\n", sys.GoVersion)
		fmt.Printf("Git Commit: %s\n", sys.GitCommit)
		fmt.Printf("OS/Arch:    %s\n", sys.OSArch)
		fmt.Printf("Built:      %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	app.Action = func(c *cli.Context) error {
		c.String("config")
		config := &Config{
			Host:    c.String("host"),
			Port:    c.Uint("port"),
			Wallet:  c.String("wallet"),
			APIPort: c.Uint("api.port"),
		}

		// start the server
		n := runServer(config, logger)

		// run the shell version of the node
		runShell(n, logger)

		return nil
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	err := app.Run(os.Args)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to parse configuration/command-line arguments.")
	}
}

func runServer(config *Config, logger zerolog.Logger) *noise.Node {
	var keys identity.Keypair

	privateKey, err := ioutil.ReadFile(config.Wallet)
	if err != nil {
		logger.Warn().Msgf("Could not find an existing wallet at %q. Generating a new wallet...", config.Wallet)

		keys = skademlia.NewKeys(DefaultC1, DefaultC2)

		logger.Info().
			Hex("privateKey", keys.PrivateKey()).
			Hex("publicKey", keys.PublicKey()).
			Msg("Generated a wallet.")
	} else {
		n, err := hex.Decode(privateKey, privateKey)
		if err != nil {
			logger.Fatal().Err(err).Msgf("Failed to decode your private key from %q.", config.Wallet)
		}

		keys, err = skademlia.LoadKeys(privateKey[:n], DefaultC1, DefaultC2)
		if err != nil {
			logger.Fatal().Err(err).Msgf("The private key specified in %q is invalid.", config.Wallet)
		}

		logger.Info().
			Hex("privateKey", keys.PrivateKey()).
			Hex("publicKey", keys.PublicKey()).
			Msg("Loaded wallet.")
	}

	params := noise.DefaultParams()
	params.Keys = keys
	params.Host = config.Host
	params.Port = uint16(config.Port)
	params.MaxMessageSize = 4 * 1024 * 1024
	params.SendMessageTimeout = 1 * time.Second

	n, err := noise.NewNode(params)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to start listening for peers.")
	}

	n.OnPeerInit(func(node *noise.Node, peer *noise.Peer) error {
		peer.OnConnError(func(node *noise.Node, peer *noise.Peer, err error) error {
			logger.Info().Err(err).Msgf("An error occurred over the wire.")

			return nil
		})

		peer.OnDisconnect(func(node *noise.Node, peer *noise.Peer) error {
			logger.Info().Msgf("Peer %v has disconnected.", peer.RemoteIP().String()+":"+strconv.Itoa(int(peer.RemotePort())))

			// TODO(kenta): don't immediately evict from table
			skademlia.Table(node).Delete(protocol.PeerID(peer))
			return nil
		})

		return nil
	})

	protocol.New().
		Register(ecdh.New()).
		Register(aead.New()).
		Register(skademlia.New().WithC1(DefaultC1).WithC2(DefaultC2)).
		Register(node.New()).
		Enforce(n)

	go n.Listen()

	logger.Info().Uint16("port", n.ExternalPort()).Msg("Listening for peers.")

	if len(flag.Args()) > 0 {
		for _, address := range flag.Args() {
			peer, err := n.Dial(address)
			if err != nil {
				logger.Fatal().Err(err).Msg("Failed to dial specified peer.")
			}

			node.WaitUntilAuthenticated(peer)
		}

		peers := skademlia.FindNode(n, protocol.NodeID(n).(skademlia.ID), skademlia.BucketSize(), 8)
		logger.Info().Msgf("Bootstrapped with peers: %+v", peers)
	}

	if port := config.APIPort; port > 0 {
		go api.New().StartHTTP(n, int(config.APIPort))
	}

	return n
}

func runShell(n *noise.Node, logger zerolog.Logger) {
	reader := bufio.NewReader(os.Stdin)

	var nodeID common.AccountID
	copy(nodeID[:], n.Keys.PublicKey())

	for {
		bytes, _, err := reader.ReadLine()

		if err != nil {
			logger.Fatal().Err(err).Msg("Failed to read input from user.")
		}

		ledger := node.Ledger(n)
		cmd := strings.Split(string(bytes), " ")

		switch cmd[0] {
		case "l":
			logger.Info().
				Uint64("difficulty", ledger.Difficulty()).
				Uint64("view_id", ledger.ViewID()).
				Hex("root_id", ledger.Root().ID[:]).
				Uint64("height", ledger.Height()).
				Msg("Here is the current state of the ledger.")
		case "w":
			if len(cmd) < 2 {
				balance, _ := ledger.ReadAccountBalance(nodeID)
				stake, _ := ledger.ReadAccountStake(nodeID)

				logger.Info().
					Str("id", hex.EncodeToString(n.Keys.PublicKey())).
					Uint64("balance", balance).
					Uint64("stake", stake).
					Msg("Here is your wallet information.")

				continue
			}

			buf, err := hex.DecodeString(cmd[1])

			if err != nil || len(buf) != common.SizeAccountID {
				logger.Error().Msg("The account ID you specified is invalid.")
				continue
			}

			var accountID common.AccountID
			copy(accountID[:], buf)

			balance, _ := ledger.ReadAccountBalance(accountID)
			stake, _ := ledger.ReadAccountStake(accountID)

			logger.Info().
				Uint64("balance", balance).
				Uint64("stake", stake).
				Msgf("Account: %s", cmd[1])
		case "p":
			recipientAddress := "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405"
			amount := 1

			if len(cmd) >= 2 {
				recipientAddress = cmd[1]
			}

			if len(cmd) >= 3 {
				amount, err = strconv.Atoi(cmd[2])
				if err != nil {
					logger.Error().Err(err).Msg("Failed to convert payment amount to an uint64.")
					continue
				}
			}

			recipient, err := hex.DecodeString(recipientAddress)
			if err != nil {
				logger.Error().Err(err).Msg("The recipient you specified is invalid.")
				continue
			}

			params := payload.NewWriter(nil)

			params.WriteBytes(recipient)
			params.WriteUint64(uint64(amount))

			if len(cmd) >= 5 {
				params.WriteString(cmd[3])

				inputs := payload.NewWriter(nil)

				for i := 4; i < len(cmd); i++ {
					arg := cmd[i]

					switch arg[0] {
					case 'S':
						inputs.WriteString(arg[1:])
					case 'B':
						inputs.WriteBytes([]byte(arg[1:]))
					case '1', '2', '4', '8':
						var val uint64
						_, err = fmt.Sscanf(arg[1:], "%d", &val)
						if err != nil {
							logger.Error().Err(err).Msgf("Got an error parsing integer: %+v", arg[1:])
						}

						switch arg[0] {
						case '1':
							inputs.WriteByte(byte(val))
						case '2':
							inputs.WriteUint16(uint16(val))
						case '4':
							inputs.WriteUint32(uint32(val))
						case '8':
							inputs.WriteUint64(uint64(val))
						}
					case 'H':
						b, err := hex.DecodeString(arg[1:])
						if err != nil {
							logger.Error().Err(err).Msgf("Cannot decode hex: %s", arg[1:])
							continue
						}

						inputs.WriteBytes(b)
					default:
						logger.Error().Msgf("Invalid argument specified: %s", arg)
						continue
					}
				}

				params.WriteBytes(inputs.Bytes())
			}

			go func() {
				tx, err := ledger.NewTransaction(n.Keys, sys.TagTransfer, params.Bytes())
				if err != nil {
					logger.Error().Err(err).Msg("Failed to create a transfer transaction.")
					return
				}

				err = node.BroadcastTransaction(n, tx)
				if err != nil {
					logger.Error().Err(err).Msg("An error occurred while broadcasting a transfer transaction.")
					return
				}

				logger.Info().Msgf("Success! Your payment transaction ID: %x", tx.ID)
			}()

		case "ps":
			if len(cmd) < 2 {
				continue
			}

			amount, err := strconv.Atoi(cmd[1])
			if err != nil {
				logger.Fatal().Err(err).Msg("Failed to convert staking amount to a uint64.")
			}

			go func() {
				tx, err := ledger.NewTransaction(n.Keys, sys.TagStake, payload.NewWriter(nil).WriteUint64(uint64(amount)).Bytes())
				if err != nil {
					logger.Error().Err(err).Msg("Failed to create a stake placement transaction.")
					return
				}

				err = node.BroadcastTransaction(n, tx)
				if err != nil {
					logger.Error().Err(err).Msg("An error occurred while broadcasting a stake placement transaction.")
					return
				}

				logger.Info().Msgf("Success! Your stake placement transaction ID: %x", tx.ID)
			}()
		case "ws":
			if len(cmd) < 2 {
				continue
			}

			amount, err := strconv.Atoi(cmd[1])
			if err != nil {
				logger.Fatal().Err(err).Msg("Failed to convert withdraw amount to an uint64.")
			}

			go func() {
				tx, err := ledger.NewTransaction(n.Keys, sys.TagStake, payload.NewWriter(nil).WriteUint64(uint64(amount)).Bytes())
				if err != nil {
					logger.Error().Err(err).Msg("Failed to create a stake withdrawal transaction.")
					return
				}

				err = node.BroadcastTransaction(n, tx)
				if err != nil {
					logger.Error().Err(err).Msg("An error occurred while broadcasting a stake withdrawal transaction.")
					return
				}

				logger.Info().Msgf("Success! Your stake withdrawal transaction ID: %x", tx.ID)
			}()
		case "c":
			if len(cmd) < 2 {
				continue
			}

			code, err := ioutil.ReadFile(cmd[1])
			if err != nil {
				logger.Error().
					Err(err).
					Str("path", cmd[1]).
					Msg("Failed to find/load the smart contract code from the given path.")
				continue
			}

			go func() {
				tx, err := ledger.NewTransaction(n.Keys, sys.TagContract, code)
				if err != nil {
					logger.Error().Err(err).Msg("Failed to create a smart contract creation transaction.")
					return
				}

				err = node.BroadcastTransaction(n, tx)
				if err != nil {
					logger.Error().Err(err).Msg("An error occurred while broadcasting a smart contract creation transaction.")
					return
				}

				logger.Info().Msgf("Success! Your smart contract ID: %x", tx.ID)
			}()
		}
	}
}