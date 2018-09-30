package main

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/noise/crypto/ed25519"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/noise/network/discovery"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/cmd/utils"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/security"
	"github.com/urfave/cli"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

func main() {
	app := cli.NewApp()

	app.Name = "wavelet"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = utils.Version
	app.Usage = "a bleeding fast ledger with a powerful compute layer"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "host, address",
			Value: "localhost",
			Usage: "Listen for peers on host address `HOST`.",
		},
		cli.UintFlag{
			Name:  "port, p",
			Value: 3000,
			Usage: "Listen for peers on port `PORT`.",
		},
		cli.UintFlag{
			Name:  "api",
			Usage: "Host a local HTTP API at port `API_PORT`.",
		},
		cli.StringFlag{
			Name:  "database, db",
			Value: "testdb",
			Usage: "Load/initialize LevelDB store from `DB_PATH`.",
		},
		cli.StringFlag{
			Name:  "services, s",
			Value: "services",
			Usage: "Load WebAssembly transaction processor services from `SERVICES_PATH`.",
		},
		cli.StringFlag{
			Name:  "privkey, sk",
			Value: "6d6fe0c2bc913c0e3e497a0328841cf4979f932e01d2030ad21e649fca8d47fe71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858",
			Usage: "Set the node's private key to be `PRIVATE_KEY`. Leave `PRIVATE_KEY` = 'random' if you want to randomly generate one.",
		},
		cli.StringSliceFlag{
			Name:  "nodes, peers, n",
			Usage: "Bootstrap to peers whose address are formatted as tcp://[host]:[port] from `PEER_NODES`.",
		},
	}

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version: %s\n", c.App.Version)
		fmt.Printf("Go Version: %s\n", utils.GoVersion)
		fmt.Printf("Git Commit: %s\n", utils.GitCommit)
		fmt.Printf("Built: %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	app.Action = func(c *cli.Context) {
		privateKey := c.String("privkey")

		if privateKey == "random" {
			privateKey = ed25519.RandomKeyPair().PrivateKeyHex()
		}

		keys, err := crypto.FromPrivateKey(security.SignaturePolicy, privateKey)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to decode private key.")
		}

		w := node.NewPlugin(node.Options{
			DatabasePath: c.String("db"),
			ServicesPath: c.String("services"),
		})

		builder := network.NewBuilder()

		builder.SetKeys(keys)
		builder.SetAddress(network.FormatAddress("tcp", c.String("host"), uint16(c.Uint("port"))))

		builder.AddPlugin(new(discovery.Plugin))
		builder.AddPlugin(w)

		net, err := builder.Build()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to initialize networking.")
		}

		go net.Listen()

		net.BlockUntilListening()

		if peers := c.StringSlice("peers"); len(peers) > 0 {
			net.Bootstrap(peers...)
		}

		if port := c.Uint("api"); port > 0 {
			go api.Run(net, api.Options{
				ListenAddr: fmt.Sprintf("%s:%d", c.String("host"), port),
				Clients: []*api.ClientInfo{
					{
						PublicKey: net.ID.PublicKeyHex(),
						Permissions: api.ClientPermissions{
							CanSendTransaction: true,
							CanPollTransaction: true,
							CanControlStats:    true,
						},
					},
				},
			})

			log.Info().
				Str("host", c.String("host")).
				Uint("port", port).
				Msg("Local HTTP API is being served.")
		}

		exit := make(chan os.Signal, 1)
		signal.Notify(exit, os.Interrupt)

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
			case "wallet":
				w.Ledger.Do(func(l *wavelet.Ledger) {
					log.Info().
						Str("id", hex.EncodeToString(w.Wallet.PublicKey)).
						Uint64("nonce", w.Wallet.CurrentNonce(l)).
						Uint64("balance", w.Wallet.GetBalance(l)).
						Msg("Here is your wallet information.")
				})
			case "pay":
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
				w.BroadcastTransaction(wired)
			case "contract":
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
					log.Fatal().Err(err).Msg("Failed to marshal smart contract deployment payload.")
				}

				wired := w.MakeTransaction("create_contract", payload)
				w.BroadcastTransaction(wired)

				contractID := hex.EncodeToString(wavelet.ContractID(graph.Symbol(wired)))

				log.Info().Msgf("Success! Your smart contract ID is: %s", contractID)
			default:
				wired := w.MakeTransaction("nop", nil)
				w.BroadcastTransaction(wired)
			}
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse configuration/command-line arugments.")
	}
}
