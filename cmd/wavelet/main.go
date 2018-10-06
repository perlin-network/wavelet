package main

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/cmd/utils"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/security"

	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/noise/crypto/ed25519"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/noise/network/discovery"

	"github.com/perlin-network/graph/database"
	"gopkg.in/urfave/cli.v1"
	"gopkg.in/urfave/cli.v1/altsrc"
)

func main() {
	app := cli.NewApp()

	app.Name = "wavelet"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = utils.Version
	app.Usage = "a bleeding fast ledger with a powerful compute layer"

	app.Flags = []cli.Flag{
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "host",
			Value: "localhost",
			Usage: "Listen for peers on host address `HOST`.",
		}),
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
		altsrc.NewStringSliceFlag(cli.StringSliceFlag{
			Name:  "api.clients.public_key",
			Value: &cli.StringSlice{"71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858"},
			Usage: "The public keys with access to your wavelet client's API `PUBLIC_KEY`",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "db.path",
			Value: "testdb",
			Usage: "Load/initialize LevelDB store from `DB_PATH`.",
		}),
		altsrc.NewBoolFlag(cli.BoolFlag{
			Name:  "db.reset",
			Usage: "Clear out the existing data in the datastore before initializing",
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
			Name:  "privkey",
			Value: "6d6fe0c2bc913c0e3e497a0328841cf4979f932e01d2030ad21e649fca8d47fe71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858",
			Usage: "Set the node's private key to be `PRIVATE_KEY`. Leave `PRIVATE_KEY` = 'random' if you want to randomly generate one.",
		}),
		altsrc.NewStringSliceFlag(cli.StringSliceFlag{
			Name:  "peers",
			Usage: "Bootstrap to peers whose address are formatted as tcp://[host]:[port] from `PEER_NODES`.",
		}),
		altsrc.NewBoolFlag(cli.BoolFlag{
			Name:  "daemon",
			Usage: "Run node in daemon mode. Daemon mode means no standard input needed.",
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
		fmt.Printf("Version: %s\n", c.App.Version)
		fmt.Printf("Go Version: %s\n", utils.GoVersion)
		fmt.Printf("Git Commit: %s\n", utils.GitCommit)
		fmt.Printf("Built: %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	app.Action = func(c *cli.Context) {
		privateKey := c.String("privkey")
		host := c.String("host")
		port := uint16(c.Uint("port"))
		databasePath := c.String("db.path")
		resetDatabase := c.Bool("db.reset")
		servicesPath := c.String("services")
		genesisPath := c.String("genesis")
		peers := c.StringSlice("peers")
		apiHost := c.String("api.host")
		apiPort := c.Uint("api.port")
		apiPublicKeys := c.StringSlice("api.clients.public_key")
		daemon := c.Bool("daemon")

		if privateKey == "random" {
			privateKey = ed25519.RandomKeyPair().PrivateKeyHex()
		}

		keys, err := crypto.FromPrivateKey(security.SignaturePolicy, privateKey)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to decode private key.")
		}

		w := node.NewPlugin(node.Options{
			DatabasePath:  databasePath,
			ServicesPath:  servicesPath,
			GenesisPath:   genesisPath,
			ResetDatabase: resetDatabase,
		})

		builder := network.NewBuilder()

		builder.SetKeys(keys)
		builder.SetAddress(network.FormatAddress("tcp", host, port))

		builder.AddPlugin(new(discovery.Plugin))
		builder.AddPlugin(w)

		net, err := builder.Build()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to initialize networking.")
		}

		go net.Listen()

		net.BlockUntilListening()

		if len(peers) > 0 {
			net.Bootstrap(peers...)
		}

		if apiPort > 0 {
			clients := []*api.ClientInfo{}
			for _, key := range apiPublicKeys {
				client := &api.ClientInfo{
					PublicKey: key,
					Permissions: api.ClientPermissions{
						CanSendTransaction: true,
						CanPollTransaction: true,
						CanControlStats:    true,
					},
				}
				clients = append(clients, client)
			}
			go api.Run(net, api.Options{
				ListenAddr: fmt.Sprintf("%s:%d", apiHost, apiPort),
				Clients:    clients,
			})

			log.Info().
				Str("host", apiHost).
				Uint("port", apiPort).
				Msg("Local HTTP API is being served.")
		}

		exit := make(chan os.Signal, 1)
		signal.Notify(exit, os.Interrupt)

		if daemon {
			<-exit

			net.Close()
			os.Exit(0)
		}

		go func() {
			<-exit

			net.Close()
			os.Exit(0)
		}()

		reader := bufio.NewReader(os.Stdout)

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
					account, err = l.LoadAccount(accountID)
				})

				if err != nil {
					log.Error().Msg("There is no account with that ID in the database.")
					continue
				}

				balance, exists := account.Load("balance")
				if !exists {
					log.Error().Msg("The account has no balance associated to it.")
					continue
				}

				log.Info().
					Uint64("nonce", account.Nonce).
					Uint64("balance", binary.LittleEndian.Uint64(balance)).
					Msgf("Account Info: %s", cmd[1])
			case "p":
				recipient := "71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858"
				amount := 1

				if len(cmd) >= 2 {
					recipient = cmd[1]
				}

				if len(cmd) >= 3 {
					amount, err = strconv.Atoi(cmd[2])
					if err != nil {
						log.Fatal().Err(err).Msg("Failed to convert payment amount to an uint64.")
					}
				}

				transfer := struct {
					Recipient string `json:"recipient"`
					Amount    uint64 `json:"amount"`
				}{
					Recipient: recipient,
					Amount:    uint64(amount),
				}

				payload, err := json.Marshal(transfer)
				if err != nil {
					log.Fatal().Err(err).Msg("Failed to marshal transfer payload.")
				}

				wired := w.MakeTransaction("transfer", payload)
				go w.BroadcastTransaction(wired)
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

				contract := struct {
					Code string `json:"code"`
				}{Code: base64.StdEncoding.EncodeToString(bytes)}

				payload, err := json.Marshal(contract)
				if err != nil {
					log.Warn().Err(err).Msg("Failed to marshal smart contract deployment payload.")
					continue
				}

				wired := w.MakeTransaction("create_contract", payload)
				go w.BroadcastTransaction(wired)

				contractID := hex.EncodeToString(wavelet.ContractID(graph.Symbol(wired)))

				log.Info().Msgf("Success! Your smart contract ID is: %s", contractID)
			case "tx":
				if len(cmd) < 2 {
					continue
				}

				var tx *database.Transaction

				w.Ledger.Do(func(l *wavelet.Ledger) {
					tx, err = l.GetBySymbol(cmd[1])
				})

				if err != nil {
					log.Error().Err(err).Msg("Unable to find transaction.")
					continue
				}

				view := struct {
					Sender    string                 `json:"sender"`
					Nonce     uint64                 `json:"nonce"`
					Tag       string                 `json:"tag"`
					Payload   map[string]interface{} `json:"payload"`
					Signature string                 `json:"signature"`
					Parents   []string               `json:"parents"`
				}{
					Sender:    tx.Sender,
					Nonce:     tx.Nonce,
					Tag:       tx.Tag,
					Payload:   make(map[string]interface{}),
					Signature: hex.EncodeToString(tx.Signature),
					Parents:   tx.Parents,
				}

				err := json.Unmarshal(tx.Payload, &view.Payload)

				if err != nil {
					log.Error().Err(err).Msg("Failed to unmarshal transactions payload.")
					continue
				}

				log.Info().Interface("tx", view).Msg("Here is the transaction you requested.")
			default:
				wired := w.MakeTransaction("nop", nil)
				go w.BroadcastTransaction(wired)
			}
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse configuration/command-line arugments.")
	}
}
