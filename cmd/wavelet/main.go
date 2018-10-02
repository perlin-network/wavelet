package main

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/cmd/utils"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/security"

	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/noise/crypto/ed25519"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/noise/network/discovery"

	"gopkg.in/urfave/cli.v1"
	"gopkg.in/urfave/cli.v1/altsrc"
)

type ClientConfig struct {
	Database   string   `toml:"database"`
	Peers      []string `toml:"peers"`
	Port       uint16   `toml:"port"`
	PrivateKey string   `toml:"private_key"`
}

type Config struct {
	API    *api.Options  `toml:"api"`
	Client *ClientConfig `toml:"client"`
}

func main() {
	app := cli.NewApp()

	app.Name = "wavelet"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = utils.Version
	app.Usage = "a bleeding fast ledger with a powerful compute layer"

	// alternate names breaks loading from a config
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
		altsrc.NewIntFlag(cli.IntFlag{
			Name:  "api.port",
			Usage: "Host a local HTTP API at port `API_PORT`.",
		}),
		altsrc.NewStringSliceFlag(cli.StringSliceFlag{
			Name:  "api.clients.public_key",
			Usage: "The public keys with access to your wavelet client's API.",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "db",
			Value: "testdb",
			Usage: "Load/initialize LevelDB store from `DB_PATH`.",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "services",
			Value: "services",
			Usage: "Load WebAssembly transaction processor services from `SERVICES_PATH`.",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "privkey",
			Value: "random",
			Usage: "Set the node's private key to be `PRIVATE_KEY`. Leave `PRIVATE_KEY` = 'random' if you want to randomly generate one.",
		}),
		altsrc.NewStringFlag(cli.StringFlag{
			Name:  "genesis",
			Value: "genesis.csv",
			Usage: "CSV file containing account data to initialize the ledger from `GENESIS_CSV`.",
		}),
		altsrc.NewStringSliceFlag(cli.StringSliceFlag{
			Name:  "peers",
			Usage: "Bootstrap to peers whose address are formatted as tcp://[host]:[port] from `PEER_NODES`.",
		}),
		cli.StringFlag{
			Name:  "config, c",
			Usage: "Wavelet configuration file.",
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
		host := c.String("host")
		port := uint16(c.Uint("port"))
		databasePath := c.String("db")
		servicesPath := c.String("services")
		genesisPath := c.String("genesis")
		peers := c.StringSlice("peers")
		apiPort := c.Uint("api.port")
		apiPublicKeys := c.StringSlice("api.clients.public_key")

		log.Info().Interface("pub_keys:", apiPublicKeys).Msg("")

		if privateKey == "random" {
			privateKey = ed25519.RandomKeyPair().PrivateKeyHex()
		}

		keys, err := crypto.FromPrivateKey(security.SignaturePolicy, privateKey)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to decode private key.")
		}

		w := node.NewPlugin(node.Options{
			DatabasePath: databasePath,
			ServicesPath: servicesPath,
			GenesisCSV:   genesisPath,
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
				ListenAddr: fmt.Sprintf("%s:%d", host, apiPort),
				Clients:    clients,
			})

			log.Info().
				Str("host", c.String("host")).
				Uint("port", apiPort).
				Msg("Local HTTP API is being served.")
		}

		exit := make(chan os.Signal, 1)
		signal.Notify(exit, os.Interrupt)

		go func() {
			<-exit

			net.Close()
			os.Exit(0)
		}()

		for {
		}
	}

	app.Before = altsrc.InitInputSourceWithContext(app.Flags, func(c *cli.Context) (altsrc.InputSourceContext, error) {
		filePath := c.String("config")
		if filePath != "" {
			return altsrc.NewTomlSourceFromFile(filePath)
		}
		return &altsrc.MapInputSource{}, nil
	})

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse configuration/command-line arugments.")
	}
}
