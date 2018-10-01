package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

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

	switch cmd[0] {
	case "wallet":
		if len(cmd) != 2 {
			log.Fatal().Msg("wallet expected 1 argument: wallet [address]")
		}

		recipient := cmd[1]
		var ret map[string][]byte
		if err := client.Request("/account/load", recipient, &ret); err != nil {
			log.Fatal().Err(err).Msg("")
		}
		log.Info().Msgf("Wallet information: %v", ret)
	case "pay":
		tag := "transfer"

		if len(cmd) != 3 {
			log.Fatal().Msg("transfer expected 2 arguments: transfer [recipient] [amount]")
		}

		recipient := cmd[1]
		amount, err := strconv.Atoi(cmd[2])
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to convert payment amount to an uint64.")
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

		if err := client.Request("/transaction/send", struct {
			Tag     string `json:"tag"`
			Payload []byte `json:"payload"`
		}{
			Tag:     tag,
			Payload: payload,
		}, nil); err != nil {
			log.Fatal().Err(err).Msg("Failed to send pay command.")
		}
	case "contract":
		if len(cmd) != 3 {
			log.Fatal().Msg("contract expected 1 argument: contract [path]")
		}

		contractPath := cmd[1]

		bytes, err := ioutil.ReadFile(contractPath)
		if err != nil {
			log.Fatal().
				Err(err).
				Str("path", contractPath).
				Msg("Failed to find/load the smart contract code from the given path.")
		}

		contract := struct {
			Code string `json:"code"`
		}{
			Code: base64.StdEncoding.EncodeToString(bytes),
		}

		payload, err := json.Marshal(contract)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to marshal smart contract deployment payload.")
		}

		log.Info().Msgf("Result: %v", payload)
	case "stats_reset":
		res := new(interface{})
		if err := client.Request("/stats/reset", struct{}{}, res); err != nil {
			log.Fatal().Err(err).Msg("")
		}
		jsonOut, _ := json.Marshal(res)
		fmt.Printf("%s\n", jsonOut)
	case "stats_summary":
		res := new(interface{})
		if err := client.Request("/stats/summary", struct{}{}, res); err != nil {
			log.Fatal().Err(err).Msg("")
		}
		jsonOut, _ := json.Marshal(res)
		fmt.Printf("%s\n", jsonOut)
	default:
		log.Fatal().Msgf("unknown command: %s", cmd[0])
	}
}
