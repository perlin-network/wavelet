package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/noise/crypto/ed25519"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/noise/network/discovery"
	cmdUtils "github.com/perlin-network/wavelet/cmd/utils"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/security"
	"github.com/urfave/cli"
	"os"
	"os/signal"
)

func main() {
	app := cli.NewApp()

	app.Name = "wavelet"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = cmdUtils.Version
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
			Name:  "privkey, pk",
			Value: "a6a193b4665b03e6df196ab7765b04a01de00e09c4a056f487019b5e3565522fd6edf02c950c6e091cd2450552a52febbb3d29b38c22bb89b0996225ef5ec972",
			Usage: "Set the node's private key to be `PRIVATE_KEY`. Leave `PRIVATE_KEY` = 'random' if you want to randomly generate one.",
		},
		cli.StringSliceFlag{
			Name:  "nodes, peers, n",
			Usage: "Bootstrap to peers whose address are formatted as tcp://[host]:[port] from `PEER_NODES`.",
		},
	}

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version: %s\n", c.App.Version)
		fmt.Printf("Go Version: %s\n", cmdUtils.GoVersion)
		fmt.Printf("Git Commit: %s\n", cmdUtils.GitCommit)
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

		wavelet := node.NewPlugin(node.Options{
			DatabasePath: c.String("db"),
			ServicesPath: c.String("services"),
		})

		builder := network.NewBuilder()

		builder.SetKeys(keys)
		builder.SetAddress(network.FormatAddress("tcp", c.String("host"), uint16(c.Uint("port"))))

		builder.AddPlugin(new(discovery.Plugin))
		builder.AddPlugin(wavelet)

		net, err := builder.Build()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to initialize networking.")
		}

		go net.Listen()

		net.BlockUntilListening()

		if peers := c.StringSlice("peers"); len(peers) > 0 {
			fmt.Println(peers)
			net.Bootstrap(peers...)
		}

		exit := make(chan os.Signal, 1)
		signal.Notify(exit, os.Interrupt)

		go func() {
			<-exit

			net.Close()
			os.Exit(0)
		}()

		reader := bufio.NewReader(os.Stdout)

		for i := 0; ; i++ {
			fmt.Print("Enter a message: ")

			bytes, _, err := reader.ReadLine()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to read line from stdin.")
			}

			switch string(bytes) {
			case "wallet":
				log.Info().
					Str("id", hex.EncodeToString(wavelet.Wallet.PublicKey)).
					Uint64("nonce", wavelet.Wallet.CurrentNonce()).
					Uint64("balance", wavelet.Wallet.GetBalance(wavelet.Ledger)).
					Msg("Here is your wallet information.")
			case "pay":
				transfer := struct {
					Recipient string `json:"recipient"`
					Amount    uint64 `json:"amount"`
				}{"71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858", 1}

				payload, err := json.Marshal(transfer)
				if err != nil {
					log.Fatal().Err(err).Msg("Failed to marshal transfer payload.")
				}

				wired := wavelet.MakeTransaction("transfer", payload)
				wavelet.BroadcastTransaction(wired)
			default:
				wired := wavelet.MakeTransaction("nop", nil)
				wavelet.BroadcastTransaction(wired)
			}
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse configuration/command-line arugments.")
	}
}
