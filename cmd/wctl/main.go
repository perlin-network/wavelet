package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/cmd/utils"
	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func main() {

	app := cli.NewApp()

	app.Name = "wctl"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = utils.Version
	app.Usage = "a cli client to interact with the wavelet node"

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version: %s\n", c.App.Version)
		fmt.Printf("Go Version: %s\n", utils.GoVersion)
		fmt.Printf("Git Commit: %s\n", utils.GitCommit)
		fmt.Printf("Built: %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	commonFlags := []cli.Flag{
		cli.StringFlag{
			Name:  "remote",
			Usage: "remote address `REMOTE` (required).",
		},
		cli.StringFlag{
			Name:  "privkey",
			Usage: "private key (hex) `KEY` (required).",
		},
	}

	app.Commands = []cli.Command{
		cli.Command{
			Name:      "send_transaction",
			Usage:     "send a transaction",
			ArgsUsage: "<tag> <json payload>",
			Flags:     commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				tag := c.Args().Get(0)
				payload := c.Args().Get(1)
				return client.SendTransaction(tag, []byte(payload))
			},
		},
		cli.Command{
			Name:  "recent_transactions",
			Usage: "get recent transactions",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				transactions, err := client.RecentTransactions()
				if err != nil {
					return err
				}
				for _, tx := range transactions {
					log.Info().Msgf("%v", tx)
				}
				return nil
			},
		},
		cli.Command{
			Name:  "poll_accounts",
			Usage: "continuously receive account updates",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				evChan, err := client.PollAccountUpdates(nil)
				if err != nil {
					return err
				}
				for ev := range evChan {
					log.Info().Msgf("%v", ev)
				}
				return nil
			},
		},
		cli.Command{
			Name:      "poll_transactions",
			Usage:     "continuously receive transaction updates",
			ArgsUsage: "<accepted | applied>",
			Flags:     commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				event := c.Args().Get(0)

				var evChan <-chan wire.Transaction
				switch event {
				case "accepted":
					evChan, err = client.PollAcceptedTransactions(nil)
				case "applied":
					evChan, err = client.PollAppliedTransactions(nil)
				default:
					return errors.Errorf("invalid event type specified: %v", event)
				}
				if err != nil {
					return err
				}

				for ev := range evChan {
					log.Info().Msgf("%v", ev)
				}
				return nil
			},
		},
		cli.Command{
			Name:  "stats_reset",
			Usage: "reset the stats counters",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				res := new(interface{})
				if err := client.StatsReset(res); err != nil {
					return err
				}
				jsonOut, _ := json.Marshal(res)
				fmt.Printf("%s\n", jsonOut)
				return nil
			},
		},
		cli.Command{
			Name:  "stats_summary",
			Usage: "get the stats counters",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				res := new(interface{})
				if err := client.StatsSummary(res); err != nil {
					return err
				}
				jsonOut, _ := json.Marshal(res)
				fmt.Printf("%s\n", jsonOut)
				return nil
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal().Err(err).Msg("Failed to parse configuration/command-line arugments.")
	}
}

func setup(c *cli.Context) (*api.Client, error) {

	remoteAddr := c.String("remote")
	privateKey := c.String("privkey")

	if len(remoteAddr) == 0 {
		return nil, errors.New("remote flag is missing")
	}

	if len(privateKey) == 0 {
		return nil, errors.New("private key is missing")
	}

	client, err := api.NewClient(api.ClientConfig{
		RemoteAddr: remoteAddr,
		PrivateKey: privateKey,
		UseHTTPS:   false,
	})
	if err != nil {
		return nil, err
	}

	err = client.Init()
	if err != nil {
		return nil, err
	}

	log.Debug().Str("SessionToken", client.SessionToken).Msg("")
	return client, nil
}
