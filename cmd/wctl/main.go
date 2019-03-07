package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"time"
)

func main() {
	app := cli.NewApp()

	app.Name = "wctl"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = sys.Version
	app.Usage = "a cli client to interact with the wavelet node"

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version:    %s\n", sys.Version)
		fmt.Printf("Go Version: %s\n", sys.GoVersion)
		fmt.Printf("Git Commit: %s\n", sys.GitCommit)
		fmt.Printf("OS/Arch:    %s\n", sys.OSArch)
		fmt.Printf("Built:      %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	commonFlags := []cli.Flag{
		cli.StringFlag{
			Name:  "api.host",
			Value: "localhost",
			Usage: "Host of the local HTTP API.",
		},
		cli.IntFlag{
			Name:  "api.port",
			Usage: "Port a local HTTP API.",
		},
		cli.StringFlag{
			Name:  "wallet",
			Usage: "path to file containing hex-encoded private key",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "poll_broadcaster",
			Usage: "continuously receive broadcaster updates",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}

				client.UseHTTPS = true
				evChan, err := client.PollLoggerSink(nil, wctl.RouteWSBroadcaster)
				if err != nil {
					return err
				}

				for ev := range evChan {
					jsonOut, _ := json.Marshal(ev)
					fmt.Printf("%s\n", jsonOut)
				}
				return nil
			},
		},
		{
			Name:  "poll_consensus",
			Usage: "continuously receive consensus updates",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}

				client.UseHTTPS = true
				evChan, err := client.PollLoggerSink(nil, wctl.RouteWSConsensus)
				if err != nil {
					return err
				}

				for ev := range evChan {
					jsonOut, _ := json.Marshal(ev)
					fmt.Printf("%s\n", jsonOut)
				}
				return nil
			},
		},
		{
			Name:  "poll_stake",
			Usage: "continuously receive stake updates",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}

				client.UseHTTPS = true
				evChan, err := client.PollLoggerSink(nil, wctl.RouteWSStake)
				if err != nil {
					return err
				}

				for ev := range evChan {
					jsonOut, _ := json.Marshal(ev)
					fmt.Printf("%s\n", jsonOut)
				}
				return nil
			},
		},
		{
			Name:  "poll_accounts",
			Usage: "continuously receive account updates",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				// TODO:
				// accountID :=

				client.UseHTTPS = true
				evChan, err := client.PollAccounts(nil, nil)
				if err != nil {
					return err
				}

				for ev := range evChan {
					jsonOut, _ := json.Marshal(ev)
					fmt.Printf("%s\n", jsonOut)
				}
				return nil
			},
		},
		{
			Name:  "poll_contracts",
			Usage: "continuously receive contract updates",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				// TODO:
				// contractID :=

				client.UseHTTPS = true
				evChan, err := client.PollContracts(nil, nil)
				if err != nil {
					return err
				}

				for ev := range evChan {
					jsonOut, _ := json.Marshal(ev)
					fmt.Printf("%s\n", jsonOut)
				}
				return nil
			},
		},
		{
			Name:  "poll_transactions",
			Usage: "continuously receive transaction updates",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				// TODO:
				// txID :=
				// senderID :=
				// creatorID :=

				client.UseHTTPS = true
				evChan, err := client.PollTransactions(nil, nil, nil, nil)
				if err != nil {
					return err
				}

				for ev := range evChan {
					jsonOut, _ := json.Marshal(ev)
					fmt.Printf("%s\n", jsonOut)
				}
				return nil
			},
		},
		{
			Name:  "ledger_status",
			Usage: "get the status of the ledger",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				// TODO: get optional parameters

				txns, err := client.GetLedgerStatus(nil, nil, nil, nil)
				if err != nil {
					return err
				}

				for _, tx := range txns {
					jsonOut, _ := json.Marshal(tx)
					fmt.Printf("%s\n", jsonOut)
				}
				return nil
			},
		},
		{
			Name:      "get_account",
			Usage:     "get an account",
			ArgsUsage: "<account ID>",
			Flags:     commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				acctID := c.Args().Get(0)

				acct, err := client.GetAccount(acctID)
				if err != nil {
					return err
				}

				jsonOut, _ := json.Marshal(acct)
				fmt.Printf("%s\n", jsonOut)
				return nil
			},
		},
		{
			Name:      "get_contract_code",
			Usage:     "get the payload of a contract",
			ArgsUsage: "<contract ID>",
			Flags:     commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				contractID := c.Args().Get(0)

				_, err = client.GetContractCode(contractID)
				if err != nil {
					return err
				}

				// TODO: process the output
				return nil
			},
		},
		{
			Name:      "get_contract_pages",
			Usage:     "get the page of a contract",
			ArgsUsage: "<contract ID> <page index>",
			Flags:     commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				contractID := c.Args().Get(0)
				// TODO: get optional page idx

				_, err = client.GetContractPages(contractID, nil)
				if err != nil {
					return err
				}

				// TODO: process the output
				return nil
			},
		},
		{
			Name:      "send_transaction",
			Usage:     "send a transaction",
			ArgsUsage: "<tag> <json payload>",
			Flags:     commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}

				tag, err := strconv.Atoi(c.Args().Get(0))
				if err != nil {
					return err
				}

				payload := c.Args().Get(1)

				tx, err := client.SendTransaction(byte(tag), []byte(payload))
				if err != nil {
					return err
				}
				jsonOut, _ := json.Marshal(tx)
				fmt.Printf("%s\n", jsonOut)
				return nil
			},
		},
		{
			Name:      "get_transaction",
			Usage:     "get a transaction",
			ArgsUsage: "<transaction ID>",
			Flags:     commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				txID := c.Args().Get(0)

				tx, err := client.GetTransaction(txID)
				if err != nil {
					return err
				}

				jsonOut, _ := json.Marshal(tx)
				fmt.Printf("%s\n", jsonOut)
				return nil
			},
		},
		{
			Name:  "list_transactions",
			Usage: "list recent transactions",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}

				// TODO: get all these optional parameters

				txns, err := client.ListTransactions(nil, nil, nil, nil)
				if err != nil {
					return err
				}

				for _, tx := range txns {
					jsonOut, _ := json.Marshal(tx)
					fmt.Printf("%s\n", jsonOut)
				}
				return nil
			},
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(os.Args); err != nil {
		fmt.Printf("Failed to parse configuration/command-line arguments: %+v\n", err)
	}
}

func setup(c *cli.Context) (*wctl.Client, error) {
	host := c.String("api.host")
	port := c.Uint("api.port")
	privateKeyFile := c.String("wallet")

	if port == 0 {
		return nil, errors.New("port is missing")
	}

	if len(privateKeyFile) == 0 {
		return nil, errors.New("private key file is missing")
	}

	privateKeyBytes, err := ioutil.ReadFile(privateKeyFile)
	rawPrivateKey, err := hex.DecodeString(string(privateKeyBytes))
	if err != nil {
		return nil, err
	}

	config := wctl.Config{
		APIHost:  host,
		APIPort:  uint16(port),
		UseHTTPS: false,
	}
	copy(config.RawPrivateKey[:], rawPrivateKey)

	client, err := wctl.NewClient(config)
	if err != nil {
		return nil, err
	}

	err = client.Init()
	if err != nil {
		return nil, err
	}

	return client, nil
}
