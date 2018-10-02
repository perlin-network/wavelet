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
	"github.com/urfave/cli"
)

func main() {

	app := cli.NewApp()

	app.Name = "wctl"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = utils.Version
	app.Usage = "a cli client to interact with the wavelet node"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "remote, r",
			Value: "localhost:3001",
			Usage: "remote address `REMOTE`.",
		},
		cli.StringFlag{
			Name:  "privkey, k",
			Value: "",
			Usage: "private key (hex) `KEY`.",
		},
	}

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version: %s\n", c.App.Version)
		fmt.Printf("Go Version: %s\n", utils.GoVersion)
		fmt.Printf("Git Commit: %s\n", utils.GitCommit)
		fmt.Printf("Built: %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	app.Action = runAction

	if err := app.Run(os.Args); err != nil {
		log.Fatal().Err(err).Msg("Failed to parse configuration/command-line arugments.")
	}
}

func runAction(c *cli.Context) {
	remoteAddr := c.String("remote")
	privateKey := c.String("privkey")

	if len(remoteAddr) == 0 {
		log.Fatal().Msg("remote flag is missing")
	}

	client, err := api.NewClient(api.ClientConfig{
		RemoteAddr: remoteAddr,
		PrivateKey: privateKey,
		UseHTTPS:   false,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	err = client.Init()
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	log.Info().Str("SessionToken", client.SessionToken).Msg("")

	cmd := ([]string)(c.Args())
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	if len(cmd) == 0 {
		log.Fatal().Msg("Missing command argument")
	}

	switch c.Args().Get(0) {
	case "send_transaction":
		tag := c.Args().Get(1)
		payload := c.Args().Get(2)
		err := client.SendTransaction(tag, []byte(payload))
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}
	case "recent_transactions":
		transactions, err := client.RecentTransactions()
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}

		for _, tx := range transactions {
			log.Info().Msgf("%v", tx)
		}
	case "poll_accounts":
		evChan, err := client.PollAccountUpdates(nil)

		if err != nil {
			log.Fatal().Err(err).Msg("")
		}

		for ev := range evChan {
			log.Info().Msgf("%v", ev)
		}
	case "poll_transactions":
		event := c.Args().Get(1)

		var evChan <-chan wire.Transaction
		switch event {
		case "accepted":
			evChan, err = client.PollAcceptedTransactions(nil)
		case "applied":
			evChan, err = client.PollAppliedTransactions(nil)
		default:
			log.Fatal().Msgf("invalid event type specified: %v", event)
		}

		if err != nil {
			log.Fatal().Err(err).Msg("")
		}

		for ev := range evChan {
			log.Info().Msgf("%v", ev)
		}
	case "stats_reset":
		res := new(interface{})
		err := client.StatsReset(res)
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}
		jsonOut, _ := json.Marshal(res)
		fmt.Printf("%s\n", jsonOut)
	default:
		log.Fatal().Msgf("unknown command: %s", cmd)
	}
}
