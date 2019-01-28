package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"io/ioutil"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/params"
	"github.com/perlin-network/wavelet/payload"
	"github.com/perlin-network/wavelet/security"

	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/graph"

	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/noise/crypto/ed25519"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/noise/network/discovery"
	"github.com/perlin-network/noise/network/nat"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
	"gopkg.in/urfave/cli.v1/altsrc"
)

// Config describes how to start the node
type Config struct {
	PrivateKeyFile     string
	Host               string
	Port               uint
	DatabasePath       string
	ResetDatabase      bool
	GenesisPath        string
	Peers              []string
	APIHost            string
	APIPort            uint
	APIPrivateKeysFile string
	Daemon             bool
	LogLevel           string
	UseNAT             bool
}

func main() {
	app := cli.NewApp()

	app.Name = "wavelet"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = params.Version
	app.Usage = "a bleeding fast ledger with a powerful compute layer"

	app.Flags = []cli.Flag{
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "host",
			Value: "localhost",
			Usage: "Listen for peers on host address `HOST`.",
		}),
		// note: use IntFlag for numbers, UintFlag don't seem to work with the toml files
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "port",
			Value: 3000,
			Usage: "Listen for peers on port `PORT`.",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "api.host",
			Value: "localhost",
			Usage: "Host a local HTTP API at host `API_HOST`.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "api.port",
			Usage: "Host a local HTTP API at port `API_PORT`.",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "api.private_keys_file",
			Value: "wallets.txt",
			Usage: "TXT file containing private keys that can make transactions through the API `API_PRIVATE_KEYS_FILE`",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "db.path",
			Value: "testdb",
			Usage: "Load/initialize LevelDB store from `DB_PATH`.",
		}),
		altsrc.NewBoolFlag(cli.BoolFlag{
			Name:  "db.reset",
			Usage: "Clear out the existing data in the datastore before initializing the DB.",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "services",
			Value: "services",
			Usage: "Load WebAssembly transaction processor services from `SERVICES_PATH`.",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "genesis",
			Value: "genesis.json",
			Usage: "JSON file containing account data to initialize the ledger from `GENESIS_FILE`.",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "private_key_file",
			Value: "wallet.txt",
			Usage: "TXT file that contain's the node's private key `PRIVATE_KEY_FILE`. Leave `PRIVATE_KEY_FILE` = 'random' if you want to randomly generate a wallet.",
		}),
		altsrc.NewStringSliceFlag(cli.StringSliceFlag{
			Name:  "peers",
			Usage: "Bootstrap to peers whose address are formatted as tcp://[host]:[port] from `PEERS`.",
		}),
		altsrc.NewBoolFlag(cli.BoolFlag{
			Name:  "daemon",
			Usage: "Run node in daemon mode. Daemon mode means no standard input needed.",
		}),
		altsrc.NewBoolFlag(cli.BoolFlag{
			Name:  "nat",
			Usage: "Use network address traversal (NAT).",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "log_level",
			Value: "info",
			Usage: "Minimum level at which logs will be printed to stdout. One of off|debug|info|warn|error|fatal `LOG_LEVEL`.",
		}),
		// from params package
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "params.consensus_k",
			Value: params.ConsensusK,
			Usage: "Consensus parameter k number of confirmations.",
		}),
		altsrc.NewFloat64Flag(cli.Float64Flag{
			Name:  "params.consensus_alpha",
			Value: float64(params.ConsensusAlpha),
			Usage: "Consensus parameter alpha for positive strong preferences. Should be between 0-1.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "params.consensus_query_timeout_ms",
			Value: params.ConsensusQueryTimeout,
			Usage: "Timeout for querying a transaction to K peers.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "params.graph_update_period_ms",
			Value: params.GraphUpdatePeriodMs,
			Usage: "Ledger graph update period in milliseconds.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "params.sync_hint_period_ms",
			Value: params.SyncHintPeriodMs,
			Usage: "Sync hint period in milliseconds.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "params.sync_hint_num_peers",
			Value: params.SyncHintNumPeers,
			Usage: "Sync hint number of peers.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "params.sync_neighbors_likelihood",
			Value: params.SyncNeighborsLikelihood,
			Usage: "Sync probability will ask nodes to see if we're missing any of its parents/children.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "params.sync_num_peers",
			Value: params.SyncNumPeers,
			Usage: "Sync number of peers that will query for missing transactions.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "params.validator_reward_depth",
			Value: params.ValidatorRewardDepth,
			Usage: "Validator reward depth.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "params.validator_reward_amount",
			Value: int(params.ValidatorRewardAmount),
			Usage: "Validator reward amount.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "params.transaction_fee_percentage",
			Value: params.TransactionFeePercentage,
			Usage: "Transaction fee percentage.",
		}),
		// config specifies the file that overrides altsrc
		cli.StringFlag{
			Name:  "config",
			Usage: "Wavelet TOML configuration file. `CONFIG_FILE`",
		},
	}

	// apply the toml before processing the flags
	app.Before = altsrc.InitInputSourceWithContext(app.Flags, func(c *cli.Context) (altsrc.InputSourceContext, error) {
		filePath := c.String("config")
		if len(filePath) > 0 {
			return altsrc.NewTomlSourceFromFile(filePath)
		}
		return &altsrc.MapInputSource{}, nil
	})

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version:    %s\n", c.App.Version)
		fmt.Printf("Go Version: %s\n", params.GoVersion)
		fmt.Printf("Git Commit: %s\n", params.GitCommit)
		fmt.Printf("OS/Arch:    %s\n", params.OSArch)
		fmt.Printf("Built:      %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	app.Action = func(c *cli.Context) error {
		config := &Config{
			PrivateKeyFile:     c.String("private_key_file"),
			Host:               c.String("host"),
			Port:               c.Uint("port"),
			DatabasePath:       c.String("db.path"),
			ResetDatabase:      c.Bool("db.reset"),
			GenesisPath:        c.String("genesis"),
			Peers:              c.StringSlice("peers"),
			APIHost:            c.String("api.host"),
			APIPort:            c.Uint("api.port"),
			APIPrivateKeysFile: c.String("api.private_keys_file"),
			Daemon:             c.Bool("daemon"),
			LogLevel:           c.String("log_level"),
			UseNAT:             c.Bool("nat"),
		}

		if c.Uint("params.consensus_k") > 0 {
			params.ConsensusK = int(c.Uint("params.consensus_k"))
		}

		if c.Float64("params.consensus_alpha") >= 0 && c.Float64("params.consensus_alpha") <= 1 {
			params.ConsensusAlpha = float32(c.Float64("params.consensus_alpha"))
		}

		if c.Uint("params.consensus_query_timeout_ms") > 0 {
			params.ConsensusQueryTimeout = int(c.Uint("params.consensus_query_timeout_ms"))
		}

		if c.Uint("params.graph_update_period_ms") > 0 {
			params.GraphUpdatePeriodMs = int(c.Uint("params.graph_update_period_ms"))
		}

		if c.Uint("params.sync_hint_period_ms") > 0 {
			params.SyncHintPeriodMs = int(c.Uint("params.sync_hint_period_ms"))
		}

		if c.Uint("params.sync_hint_num_peers") >= 0 {
			params.SyncHintNumPeers = int(c.Uint("params.sync_hint_num_peers"))
		}

		if c.Uint("params.sync_neighbors_likelihood") > 0 {
			params.SyncNeighborsLikelihood = int(c.Uint("params.sync_neighbors_likelihood"))
		}

		if c.Uint("params.sync_num_peers") >= 0 {
			params.SyncNumPeers = int(c.Uint("params.sync_num_peers"))
		}

		if c.Uint("params.validator_reward_depth") >= 0 {
			params.ValidatorRewardDepth = int(c.Uint("params.validator_reward_depth"))
		}

		if c.Uint64("params.validator_reward_amount") >= 0 {
			params.ValidatorRewardAmount = c.Uint64("params.validator_reward_amount")
		}

		if c.Uint("params.transaction_fee_percentage") >= 0 {
			params.TransactionFeePercentage = int(c.Uint("params.transaction_fee_percentage"))
		}

		// start the plugin
		w, err := runServer(config)
		if err != nil {
			return err
		}

		// run the shell version of the node
		runShell(w)

		return nil
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse configuration/command-line arugments.")
	}
}

func runServer(c *Config) (*node.Wavelet, error) {
	log.SetLevel(c.LogLevel)

	jsonConfig, _ := json.MarshalIndent(c, "", "  ")
	log.Debug().Msgf("Config: %s", string(jsonConfig))
	log.Debug().Msgf("Params: %s", params.DumpParams())

	var privateKeyHex string
	if len(c.PrivateKeyFile) > 0 && c.PrivateKeyFile != "random" {
		bytes, err := ioutil.ReadFile(c.PrivateKeyFile)
		if err != nil {
			return nil, errors.Wrapf(err, "Unable to open server private key file: %s", c.PrivateKeyFile)
		}
		privateKeyHex = strings.TrimSpace(string(bytes))
	} else {
		log.Info().Msg("Generating a random wallet")
		privateKeyHex = ed25519.RandomKeyPair().PrivateKeyHex()
	}

	keys, err := crypto.FromPrivateKey(security.SignaturePolicy, privateKeyHex)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to decode server private key")
	}

	w := node.NewPlugin(node.Options{
		DatabasePath:  c.DatabasePath,
		GenesisPath:   c.GenesisPath,
		ResetDatabase: c.ResetDatabase,
	})

	builder := network.NewBuilder()

	builder.SetKeys(keys)
	builder.SetAddress(network.FormatAddress("tcp", c.Host, uint16(c.Port)))

	builder.AddPlugin(new(discovery.Plugin))
	if c.UseNAT {
		nat.RegisterPlugin(builder)
	}
	builder.AddPlugin(w)

	net, err := builder.Build()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize networking.")
	}

	go net.Listen()

	net.BlockUntilListening()

	if len(c.Peers) > 0 {
		net.Bootstrap(c.Peers...)
	}

	if c.APIPort > 0 {
		var clients []*api.ClientInfo

		if len(c.APIPrivateKeysFile) > 0 {
			bytes, err := ioutil.ReadFile(c.APIPrivateKeysFile)
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to open api private keys file: %s", c.APIPrivateKeysFile)
			}
			privateKeysHex := strings.Split(string(bytes), "\n")
			for i, privateKeyHex := range privateKeysHex {
				trimmed := strings.TrimSpace(privateKeyHex)
				if len(trimmed) == 0 {
					continue
				}
				keys, err := crypto.FromPrivateKey(security.SignaturePolicy, trimmed)
				if err != nil {
					log.Info().Msgf("Unable to decode key %d from file %s", i, c.APIPrivateKeysFile)
					continue
				}
				clients = append(clients, &api.ClientInfo{
					PublicKey: keys.PublicKeyHex(),
					Permissions: api.ClientPermissions{
						CanSendTransaction: true,
						CanPollTransaction: true,
						CanControlStats:    true,
						CanAccessLedger:    true,
					},
				})
			}
		}
		go api.Run(net, api.Options{
			ListenAddr: fmt.Sprintf("%s:%d", c.APIHost, c.APIPort),
			Clients:    clients,
		})

		log.Info().
			Str("host", c.APIHost).
			Uint("port", c.APIPort).
			Msg("Local HTTP API is being served.")
	}

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt)

	if c.Daemon {
		<-exit

		net.Close()
		os.Exit(0)
	}

	go func() {
		<-exit

		net.Close()
		os.Exit(0)
	}()

	return w, nil
}

func runShell(w *node.Wavelet) error {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Enter a message: ")

		bytes, _, err := reader.ReadLine()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to read line from stdin.")
		}

		cmd := strings.Split(string(bytes), " ")

		switch cmd[0] {
		case "w":
			if len(cmd) < 2 {
				w.Ledger.Do(func(l *wavelet.Ledger) {
					log.Info().
						Str("id", hex.EncodeToString(w.Wallet.PublicKey)).
						Uint64("nonce", w.Wallet.CurrentNonce(l)).
						Uint64("balance", w.Wallet.GetBalance(l)).
						Uint64("stake", w.Wallet.GetStake(l)).
						Msg("Here is your wallet information.")
				})

				continue
			}

			accountID, err := hex.DecodeString(cmd[1])

			if err != nil {
				log.Error().Msg("The account ID you specified is invalid.")
				continue
			}

			var account *wavelet.Account

			w.Ledger.Do(func(l *wavelet.Ledger) {
				account = wavelet.LoadAccount(l.Accounts, accountID)
			})

			if err != nil {
				log.Error().Msg("There is no account with that ID in the database.")
				continue
			}

			log.Info().
				Uint64("nonce", account.GetNonce()).
				Uint64("balance", account.GetBalance()).
				Uint64("stake", account.GetStake()).
				Msgf("Account: %s", cmd[1])
		case "p":
			recipient := "71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858"
			amount := 1

			if len(cmd) >= 2 {
				recipient = cmd[1]
			}

			if len(cmd) >= 3 {
				amount, err = strconv.Atoi(cmd[2])
				if err != nil {
					log.Error().Err(err).Msg("Failed to convert payment amount to an uint64.")
					continue
				}
			}

			recipientDecoded, err := hex.DecodeString(recipient)
			if err != nil {
				log.Error().Err(err).Msg("cannot decode recipient")
				continue
			}

			payload := payload.NewWriter(nil)
			payload.WriteBytes(recipientDecoded)
			payload.WriteUint64(uint64(amount))

			if len(cmd) >= 5 {
				payload.WriteString(cmd[3])

				for i := 4; i < len(cmd); i++ {
					arg := cmd[i]

					switch arg[0] {
					case 'S':
						payload.WriteString(arg[1:])
					case 'B':
						payload.WriteBytes([]byte(arg[1:]))
					case '1', '2', '4', '8':
						var val uint64
						fmt.Sscanf(arg[1:], "%d", &val)

						switch arg[0] {
						case '1':
							payload.WriteByte(byte(val))
						case '2':
							payload.WriteUint16(uint16(val))
						case '4':
							payload.WriteUint32(uint32(val))
						case '8':
							payload.WriteUint64(uint64(val))
						}
					case 'H':
						b, err := hex.DecodeString(arg[1:])
						if err != nil {
							log.Fatal().Err(err).Msgf("cannot decode hex: %s", arg[1:])
						}

						payload.WriteBytes(b)
					default:
						log.Fatal().Msgf("invalid arg: %s", arg)
					}
				}
			}

			wired := w.MakeTransaction(params.TagGeneric, payload.Bytes())
			go w.BroadcastTransaction(wired)

			log.Info().Msgf("Success! Your payment transaction ID: %s", hex.EncodeToString(graph.Symbol(wired)))
		case "ps":
			if len(cmd) < 2 {
				continue
			}

			amount, err := strconv.Atoi(cmd[1])
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to convert staking amount to a uint64.")
			}

			wired := w.MakeTransaction(params.TagStake, payload.NewWriter(nil).WriteUint64(uint64(amount)).Bytes())
			go w.BroadcastTransaction(wired)

			log.Info().Msgf("Success! Your stake placement transaction ID: %s", hex.EncodeToString(graph.Symbol(wired)))
		case "ws":
			if len(cmd) < 2 {
				continue
			}

			amount, err := strconv.Atoi(cmd[1])
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to convert withdraw amount to an uint64.")
			}

			wired := w.MakeTransaction(params.TagStake, payload.NewWriter(nil).WriteUint64(uint64(amount)).Bytes())
			go w.BroadcastTransaction(wired)

			log.Info().Msgf("Success! Your stake withdrawal transaction ID: %s", hex.EncodeToString(graph.Symbol(wired)))
		case "c":
			if len(cmd) < 2 {
				continue
			}

			bytes, err := ioutil.ReadFile(cmd[1])
			if err != nil {
				log.Error().
					Err(err).
					Str("path", cmd[1]).
					Msg("Failed to find/load the smart contract code from the given path.")
				continue
			}

			wired := w.MakeTransaction(params.TagCreateContract, bytes)
			go w.BroadcastTransaction(wired)

			log.Info().Msgf("Success! Your smart contract ID: %s", hex.EncodeToString(graph.Symbol(wired)))
		case "tx":
			if len(cmd) < 2 {
				continue
			}

			var tx *database.Transaction

			w.Ledger.Do(func(l *wavelet.Ledger) {
				tx, err = l.GetBySymbol([]byte(cmd[1]))
			})

			if err != nil {
				log.Error().Err(err).Msg("Unable to find transaction.")
				continue
			}

			var txParents []string
			for _, parent := range tx.Parents {
				txParents = append(txParents, string(parent))
			}

			log.Info().
				Str("sender", hex.EncodeToString(tx.Sender)).
				Uint64("nonce", tx.Nonce).
				Uint32("tag", tx.Tag).
				Strs("parents", txParents).
				Bytes("payload", tx.Payload).
				Msg("Here is the transaction you requested.")
		default:
			wired := w.MakeTransaction(params.TagNop, nil)
			go w.BroadcastTransaction(wired)

			log.Info().Msgf("Your nop transaction ID: %s", hex.EncodeToString(graph.Symbol(wired)))
		}
	}

	return nil
}
