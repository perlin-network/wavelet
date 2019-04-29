package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/handshake"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/noise/xnoise"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/rs/zerolog"
	"gopkg.in/urfave/cli.v1"
	"gopkg.in/urfave/cli.v1/altsrc"
	"io"
	"io/ioutil"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host     string
	Port     uint
	Wallet   string
	APIPort  uint
	Peers    []string
	Database string
}

func protocol(n *noise.Node, config *Config, keys *skademlia.Keypair, kv store.KV) (*node.Protocol, *skademlia.Protocol, noise.Protocol) {
	ecdh := handshake.NewECDH()
	ecdh.RegisterOpcodes(n)

	aead := cipher.NewAEAD()
	aead.RegisterOpcodes(n)

	overlay := skademlia.New(net.JoinHostPort(config.Host, strconv.Itoa(n.Addr().(*net.TCPAddr).Port)), keys, xnoise.DialTCP)
	overlay.RegisterOpcodes(n)
	overlay.WithC1(sys.SKademliaC1)
	overlay.WithC2(sys.SKademliaC2)

	w := node.New(overlay, keys, kv)
	w.RegisterOpcodes(n)
	w.Init(n)

	protocol := noise.NewProtocol(xnoise.LogErrors, ecdh.Protocol(), aead.Protocol(), overlay.Protocol(), w.Protocol())

	return w, overlay, protocol
}

func main() {
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(1)

	//if terminal.IsTerminal(int(os.Stdout.Fd())) {
	log.Register(log.NewConsoleWriter(log.FilterFor(log.ModuleNode, log.ModuleNetwork, log.ModuleSync, log.ModuleConsensus, log.ModuleContract)))
	//} else {
	//	log.Register(os.Stderr)
	//}

	logger := log.Node()

	app := cli.NewApp()

	app.Name = "wavelet"
	app.Author = "Perlin"
	app.Email = "support@perlin.net"
	app.Version = sys.Version
	app.Usage = "a bleeding fast ledger with a powerful compute layer"

	app.Flags = []cli.Flag{
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
		altsrc.NewStringFlag(cli.StringFlag{
			Name:   "wallet",
			Value:  "config/wallet.txt",
			Usage:  "path to file containing hex-encoded private key. If empty, a random wallet will be generated.",
			EnvVar: "WAVELET_WALLET_PATH",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:   "api.port",
			Value:  0,
			Usage:  "Host a local HTTP API at port.",
			EnvVar: "WAVELET_API_PORT",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "db",
			Value: "db",
			Usage: "Database directory",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.query_timeout",
			Value: int(sys.QueryTimeout.Seconds()),
			Usage: "Timeout in seconds for querying a transaction to K peers.",
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.max_eligible_parents_depth_diff",
			Value: sys.MaxEligibleParentsDepthDiff,
			Usage: "Max graph depth difference to search for eligible transaction parents from for our node.",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.median_timestamp_num_ancestors",
			Value: sys.MedianTimestampNumAncestors,
			Usage: "Number of ancestors to derive a median timestamp from.",
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.transaction_fee_amount",
			Value: sys.TransactionFeeAmount,
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.expected_consensus_time",
			Value: sys.ExpectedConsensusTimeMilliseconds,
			Usage: "a hardcoded consensus time in milliseconds used to compute difficulty",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.critical_timestamp_average_window_size",
			Value: sys.CriticalTimestampAverageWindowSize,
		}),
		altsrc.NewUint64Flag(cli.Uint64Flag{
			Name:  "sys.min_stake",
			Value: sys.MinimumStake,
			Usage: "minimum stake to garner validator rewards and have importance in consensus",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:   "sys.snowball.query.k",
			Value:  sys.SnowballQueryK,
			EnvVar: "SNOWBALL_QUERY_K",
			Usage:  "Snowball consensus protocol parameter k for querying.",
		}),
		altsrc.NewFloat64Flag(cli.Float64Flag{
			Name:  "sys.snowball.query.alpha",
			Value: sys.SnowballQueryAlpha,
			Usage: "Snowball consensus protocol parameter alpha",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.snowball.query.beta",
			Value: sys.SnowballQueryBeta,
			Usage: "Snowball consensus protocol parameter beta",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.snowball.sync.k",
			Value: sys.SnowballSyncK,
			Usage: "Snowball consensus protocol parameter k",
		}),
		altsrc.NewFloat64Flag(cli.Float64Flag{
			Name:  "sys.snowball.sync.alpha",
			Value: sys.SnowballSyncAlpha,
			Usage: "Snowball consensus protocol parameter alpha",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.snowball.sync.beta",
			Value: sys.SnowballSyncBeta,
			Usage: "Snowball consensus protocol parameter beta",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.difficulty.min",
			Value: sys.MinDifficulty,
			Usage: "Maximum difficulty to define a critical transaction",
		}),
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "sys.difficulty.max",
			Value: sys.MaxDifficulty,
			Usage: "Minimum difficulty to define a critical transaction",
		}),
		cli.StringFlag{
			Name:  "config, c",
			Usage: "Path to TOML config file, will override the arguments.",
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
		fmt.Printf("Go Version: %s\n", sys.GoVersion)
		fmt.Printf("Git Commit: %s\n", sys.GitCommit)
		fmt.Printf("OS/Arch:    %s\n", sys.OSArch)
		fmt.Printf("Built:      %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	app.Action = func(c *cli.Context) error {
		c.String("config")
		config := &Config{
			Host:     c.String("host"),
			Port:     c.Uint("port"),
			Wallet:   c.String("wallet"),
			APIPort:  c.Uint("api.port"),
			Peers:    c.Args(),
			Database: c.String("db"),
		}

		// set the the sys variables
		sys.SnowballQueryK = c.Int("sys.snowball.query.k")
		sys.SnowballQueryAlpha = c.Float64("sys.snowball.query.alpha")
		sys.SnowballQueryBeta = c.Int("sys.snowball.query.beta")
		sys.SnowballSyncK = c.Int("sys.snowball.sync.k")
		sys.SnowballSyncAlpha = c.Float64("sys.snowball.sync.alpha")
		sys.SnowballSyncBeta = c.Int("sys.snowball.sync.beta")
		sys.QueryTimeout = time.Duration(c.Int("sys.query_timeout")) * time.Second
		sys.MaxEligibleParentsDepthDiff = c.Uint64("sys.max_eligible_parents_depth_diff")
		sys.MinDifficulty = c.Int("sys.difficulty.min")
		sys.MaxDifficulty = c.Int("sys.difficulty.max")
		sys.MedianTimestampNumAncestors = c.Int("sys.median_timestamp_num_ancestors")
		sys.TransactionFeeAmount = c.Uint64("sys.transaction_fee_amount")
		sys.ExpectedConsensusTimeMilliseconds = c.Uint64("sys.expected_consensus_time")
		sys.CriticalTimestampAverageWindowSize = c.Int("sys.critical_timestamp_average_window_size")
		sys.MinimumStake = c.Uint64("sys.min_stake")

		// start the server
		k, n, w := server(config, logger)

		// run the shell version of the node
		shell(n, k, w, logger)

		return nil
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	err := app.Run(os.Args)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to parse configuration/command-line arguments.")
	}
}

func server(config *Config, logger zerolog.Logger) (*skademlia.Keypair, *noise.Node, *node.Protocol) {
	n, err := xnoise.ListenTCP(uint(config.Port))

	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to start listening for peers.")
		return nil, nil, nil
	}

	var k *skademlia.Keypair

	privateKeyBuf, err := ioutil.ReadFile(config.Wallet)

	if err != nil {
		logger.Warn().Msgf("Could not find an existing wallet at %q. Generating a new wallet...", config.Wallet)

		k, err = skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)

		if err != nil {
			logger.Fatal().Err(err).Msg("failed to generate a new wallet.")
			return nil, nil, nil
		}

		privateKey, publicKey := k.PrivateKey(), k.PublicKey()

		logger.Info().
			Hex("privateKey", privateKey[:]).
			Hex("publicKey", publicKey[:]).
			Msg("Generated a wallet.")
	} else {
		var privateKey edwards25519.PrivateKey

		n, err := hex.Decode(privateKey[:], privateKeyBuf)
		if err != nil {
			logger.Fatal().Err(err).Msgf("Failed to decode your private key from %q.", config.Wallet)
			return nil, nil, nil
		}

		if n != edwards25519.SizePrivateKey {
			logger.Fatal().Msgf("Private key located in %q is not of the right length.", config.Wallet)
			return nil, nil, nil
		}

		k, err = skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
		if err != nil {
			logger.Fatal().Err(err).Msgf("The private key specified in %q is invalid.", config.Wallet)
			return nil, nil, nil
		}

		privateKey, publicKey := k.PrivateKey(), k.PublicKey()

		logger.Info().
			Hex("privateKey", privateKey[:]).
			Hex("publicKey", publicKey[:]).
			Msg("Loaded wallet.")
	}

	kv, err := store.NewLevelDB(config.Database)
	if err != nil {
		logger.Fatal().Err(err).Msgf("Failed to open database %s.", config.Database)
	}

	w, network, protocol := protocol(n, config, k, kv)
	n.FollowProtocol(protocol)

	logger.Info().Str("host", config.Host).Uint("port", uint(n.Addr().(*net.TCPAddr).Port)).Msg("Listening for peers.")

	if len(config.Peers) > 0 {
		for _, address := range config.Peers {
			peer, err := xnoise.DialTCP(n, address)

			if err != nil {
				logger.Error().Err(err).Msg("Failed to dial specified peer.")
				continue
			}

			peer.WaitFor(node.SignalAuthenticated)
		}

	}

	if peers := network.Bootstrap(n); len(peers) > 0 {
		var ids []string

		for _, id := range peers {
			ids = append(ids, id.String())
		}

		logger.Info().Msgf("Bootstrapped with peers: %+v", ids)
	}

	if port := config.APIPort; port > 0 {
		go api.New().StartHTTP(int(config.APIPort), n, w.Ledger(), network, k)
	}

	return k, n, w
}

func shell(n *noise.Node, k *skademlia.Keypair, w *node.Protocol, logger zerolog.Logger) {
	publicKey := k.PublicKey()
	ledger := w.Ledger()

	reader := bufio.NewReader(os.Stdin)

	var intBuf [8]byte

	for {
		buf, _, err := reader.ReadLine()

		if err != nil {
			if err == io.EOF {
				break
			}

			continue
		}

		cmd := strings.Split(string(buf), " ")

		switch cmd[0] {
		case "l":
			preferredID := "N/A"

			if preferred := ledger.Preferred(); preferred != nil {
				preferredID = hex.EncodeToString(preferred.Root.ID[:])
			}

			round := ledger.LastRound()

			var ids []string

			for _, peer := range w.Network().Peers(n) {
				if id := peer.Ctx().Get(skademlia.KeyID); id != nil {
					ids = append(ids, id.(*skademlia.ID).String())
				}
			}

			logger.Info().
				Uint8("difficulty", round.Root.ExpectedDifficulty(byte(sys.MinDifficulty))).
				Uint64("round", round.Index).
				Hex("root_id", round.Root.ID[:]).
				Uint64("height", ledger.Height()).
				Uint64("num_tx", ledger.NumTransactions()).
				Str("preferred_id", preferredID).
				Strs("peers", ids).
				Msg("Here is the current state of the ledger.")
		case "tx":
			if len(cmd) < 2 {
				logger.Error().Msg("Please specify a transaction ID.")
			}

			buf, err := hex.DecodeString(cmd[1])

			if err != nil || len(buf) != common.SizeTransactionID {
				logger.Error().Msg("The transaction ID you specified is invalid.")
				continue
			}

			var id common.TransactionID
			copy(id[:], buf)

			tx, exists := ledger.FindTransaction(id)
			if !exists {
				logger.Error().Msg("Could not find transaction in the ledger.")
				continue
			}

			var parents []string
			for _, parentID := range tx.ParentIDs {
				parents = append(parents, hex.EncodeToString(parentID[:]))
			}

			logger.Info().
				Strs("parents", parents).
				Hex("sender", tx.Sender[:]).
				Hex("creator", tx.Creator[:]).
				Uint64("nonce", tx.Nonce).
				Uint8("tag", tx.Tag).
				Uint64("depth", tx.Depth).
				Uint64("num_ancestors", tx.Confidence).
				Msgf("Transaction: %s", cmd[1])
		case "w":
			snapshot := ledger.Snapshot()

			if len(cmd) < 2 {
				balance, _ := wavelet.ReadAccountBalance(snapshot, publicKey)
				stake, _ := wavelet.ReadAccountStake(snapshot, publicKey)
				nonce, _ := wavelet.ReadAccountNonce(snapshot, publicKey)

				logger.Info().
					Str("id", hex.EncodeToString(publicKey[:])).
					Uint64("balance", balance).
					Uint64("stake", stake).
					Uint64("nonce", nonce).
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

			balance, _ := wavelet.ReadAccountBalance(snapshot, accountID)
			stake, _ := wavelet.ReadAccountStake(snapshot, accountID)
			nonce, _ := wavelet.ReadAccountNonce(snapshot, accountID)

			_, isContract := wavelet.ReadAccountContractCode(snapshot, accountID)
			numPages, _ := wavelet.ReadAccountContractNumPages(snapshot, accountID)

			logger.Info().
				Uint64("balance", balance).
				Uint64("stake", stake).
				Uint64("nonce", nonce).
				Bool("is_contract", isContract).
				Uint64("num_pages", numPages).
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

			payload := bytes.NewBuffer(nil)
			payload.Write(recipient[:])
			binary.LittleEndian.PutUint64(intBuf[:], uint64(amount))
			payload.Write(intBuf[:])

			if len(cmd) >= 5 {
				binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(cmd[3])))
				payload.Write(intBuf[:4])
				payload.WriteString(cmd[3])

				params := bytes.NewBuffer(nil)

				for i := 4; i < len(cmd); i++ {
					arg := cmd[i]

					switch arg[0] {
					case 'S':
						binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(arg[1:])))
						params.Write(intBuf[:4])
						params.WriteString(arg[1:])
					case 'B':
						binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(arg[1:])))
						params.Write(intBuf[:4])
						params.Write([]byte(arg[1:]))
					case '1', '2', '4', '8':
						var val uint64
						_, err = fmt.Sscanf(arg[1:], "%d", &val)
						if err != nil {
							logger.Error().Err(err).Msgf("Got an error parsing integer: %+v", arg[1:])
						}

						switch arg[0] {
						case '1':
							params.WriteByte(byte(val))
						case '2':
							binary.LittleEndian.PutUint16(intBuf[:2], uint16(val))
							params.Write(intBuf[:2])
						case '4':
							binary.LittleEndian.PutUint32(intBuf[:4], uint32(val))
							params.Write(intBuf[:4])
						case '8':
							binary.LittleEndian.PutUint64(intBuf[:8], uint64(val))
							params.Write(intBuf[:8])
						}
					case 'H':
						buf, err := hex.DecodeString(arg[1:])

						if err != nil {
							logger.Error().Err(err).Msgf("Cannot decode hex: %s", arg[1:])
							continue
						}

						binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(buf)))
						params.Write(intBuf[:4])
						params.Write(buf)
					default:
						logger.Error().Msgf("Invalid argument specified: %s", arg)
						continue
					}
				}

				buf := params.Bytes()

				binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(buf)))
				payload.Write(intBuf[:4])
				payload.Write(buf)
			}

			go func() {
				tx := wavelet.NewTransaction(k, sys.TagTransfer, payload.Bytes())

				evt := wavelet.EventBroadcast{
					Tag:       tx.Tag,
					Payload:   tx.Payload,
					Creator:   tx.Creator,
					Signature: tx.CreatorSignature,
					Result:    make(chan wavelet.Transaction, 1),
					Error:     make(chan error, 1),
				}

				select {
				case <-time.After(1 * time.Second):
					logger.Info().Msg("It looks like the broadcasting queue is full. Please try again.")
					return
				case ledger.BroadcastQueue <- evt:
				}

				select {
				case <-time.After(1 * time.Second):
					logger.Info().Msg("It looks like it's taking too long to broadcast your transaction. Please try again.")
					return
				case err := <-evt.Error:
					logger.Error().Err(err).Msg("An error occurred while broadcasting a transfer transaction.")
					return
				case tx := <-evt.Result:
					logger.Info().Msgf("Success! Your payment transaction ID: %x", tx.ID)
				}
			}()

		case "ps":
			if len(cmd) < 2 {
				continue
			}

			amount, err := strconv.Atoi(cmd[1])
			if err != nil {
				logger.Error().Err(err).Msg("Failed to convert staking amount to a uint64.")
				return
			}

			payload := bytes.NewBuffer(nil)
			payload.WriteByte(1)
			binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
			payload.Write(intBuf[:8])

			go func() {
				tx := wavelet.NewTransaction(k, sys.TagStake, payload.Bytes())

				evt := wavelet.EventBroadcast{
					Tag:       tx.Tag,
					Payload:   tx.Payload,
					Creator:   tx.Creator,
					Signature: tx.CreatorSignature,
					Result:    make(chan wavelet.Transaction, 1),
					Error:     make(chan error, 1),
				}

				select {
				case <-time.After(1 * time.Second):
					logger.Info().Msg("It looks like the broadcasting queue is full. Please try again.")
					return
				case ledger.BroadcastQueue <- evt:
				}

				select {
				case <-time.After(1 * time.Second):
					logger.Info().Msg("It looks like it's taking too long to broadcast your transaction. Please try again.")
					return
				case err := <-evt.Error:
					logger.Error().Err(err).Msg("An error occurred while broadcasting a stake placement transaction.")
					return
				case tx := <-evt.Result:
					logger.Info().Msgf("Success! Your stake placement transaction ID: %x", tx.ID)
				}
			}()
		case "ws":
			if len(cmd) < 2 {
				continue
			}

			amount, err := strconv.ParseUint(cmd[1], 10, 64)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to convert withdraw amount to an uint64.")
				return
			}

			payload := bytes.NewBuffer(nil)
			payload.WriteByte(0)
			binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
			payload.Write(intBuf[:8])

			go func() {
				tx := wavelet.NewTransaction(k, sys.TagStake, payload.Bytes())

				evt := wavelet.EventBroadcast{
					Tag:       tx.Tag,
					Payload:   tx.Payload,
					Creator:   tx.Creator,
					Signature: tx.CreatorSignature,
					Result:    make(chan wavelet.Transaction, 1),
					Error:     make(chan error, 1),
				}

				select {
				case <-time.After(1 * time.Second):
					logger.Info().Msg("It looks like the broadcasting queue is full. Please try again.")
					return
				case ledger.BroadcastQueue <- evt:
				}

				select {
				case <-time.After(1 * time.Second):
					logger.Info().Msg("It looks like it's taking too long to broadcast your transaction. Please try again.")
					return
				case err := <-evt.Error:
					logger.Error().Err(err).Msg("An error occurred while broadcasting a stake withdrawal transaction.")
					return
				case tx := <-evt.Result:
					logger.Info().Msgf("Success! Your stake withdrawal transaction ID: %x", tx.ID)
				}
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
				tx := wavelet.NewTransaction(k, sys.TagContract, code)

				evt := wavelet.EventBroadcast{
					Tag:       tx.Tag,
					Payload:   tx.Payload,
					Creator:   tx.Creator,
					Signature: tx.CreatorSignature,
					Result:    make(chan wavelet.Transaction, 1),
					Error:     make(chan error, 1),
				}

				select {
				case <-time.After(1 * time.Second):
					logger.Info().Msg("It looks like the broadcasting queue is full. Please try again.")
					return
				case ledger.BroadcastQueue <- evt:
				}

				select {
				case <-time.After(1 * time.Second):
					logger.Info().Msg("It looks like it's taking too long to broadcast your transaction. Please try again.")
					return
				case err := <-evt.Error:
					logger.Error().Err(err).Msg("An error occurred while broadcasting a smart contract creation transaction.")
					return
				case tx := <-evt.Result:
					logger.Info().Msgf("Success! Your smart contracts ID is: %x", tx.ID)
				}
			}()
		}
	}
}
