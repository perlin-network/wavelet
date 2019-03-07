package main

import (
	"encoding/json"
	"fmt"
	"github.com/perlin-network/wavelet/log"
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

	logger := log.Node()
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
		cli.IntFlag{
			Name:  "p",
			Usage: "port to host HTTP API on",
		},
		cli.StringFlag{
			Name:  "w",
			Usage: "path to file containing hex-encoded private key",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "server_version",
			Usage: "get the version information of the api server",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				res, err := client.ServerVersion()
				if err != nil {
					return err
				}
				jsonOut, _ := json.Marshal(res)
				fmt.Printf("%s\n", jsonOut)
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
				tx, err := client.SendTransaction(uint32(tag), []byte(payload))
				if err != nil {
					return err
				}
				jsonOut, _ := json.Marshal(tx)
				fmt.Printf("%s\n", jsonOut)
				return nil
			},
		},
		{
			Name:  "recent_transactions",
			Usage: "get recent transactions",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				transactions, err := client.RecentTransactions(nil)
				if err != nil {
					return err
				}
				for _, tx := range transactions {
					jsonOut, _ := json.Marshal(tx)
					fmt.Printf("%s\n", jsonOut)
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
					logger.Info().Msgf("%v", ev)
				}
				return nil
			},
		},
		{
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
					logger.Info().Msgf("%v", ev)
				}
				return nil
			},
		},
		{
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
				logger.Info().Msgf("%s", string(jsonOut))
				return nil
			},
		},
		{
			Name:      "send_contract",
			Usage:     "send a smart contract",
			Flags:     commonFlags,
			ArgsUsage: "<contract_filename>",
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				filename := c.Args().Get(0)
				tx, err := client.SendContract(filename)
				if err != nil {
					return err
				}
				jsonOut, _ := json.Marshal(tx)
				fmt.Printf("%s\n", jsonOut)
				return nil
			},
		},
		{
			Name:      "get_contract",
			Usage:     "get smart contract by transaction ID",
			Flags:     commonFlags,
			ArgsUsage: "<transaction_id> <output_filename>",
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}

				contractID := c.Args().Get(0)
				filename := c.Args().Get(1)
				if _, err = client.GetContract(contractID, filename); err != nil {
					return err
				}
				logger.Info().Msgf("saved contract %s to file %s", contractID, filename)
				return nil
			},
		},
		{
			Name:  "list_contracts",
			Usage: "lists the most recent smart contracts",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}
				contracts, err := client.ListContracts(nil, nil)
				if err != nil {
					return err
				}
				fmt.Println("Contract IDs:")
				if len(contracts) == 0 {
					fmt.Println("    none found")
				} else {
					for i, contract := range contracts {
						fmt.Printf(" %d) %s\n", i+1, contract.TransactionID)
					}
				}
				return nil
			},
		},
		{
			Name:  "execute_contract",
			Usage: "executes a contract",
			Flags: commonFlags,
			Action: func(c *cli.Context) error {
				client, err := setup(c)
				if err != nil {
					return err
				}

				contractID := c.Args().Get(0)
				entry := c.Args().Get(1)
				param := c.Args().Get(2)

				result, err := client.ExecuteContract(contractID, entry, []byte(param))
				if err != nil {
					return err
				}

				fmt.Println(string(result.Result))
				return nil
			},
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(os.Args); err != nil {
		logger.Error().Msgf("Failed to parse configuration/command-line arguments: %v", err)
	}
}

func setup(c *cli.Context) (*wctl.Client, error) {
	host := "localhost"
	port := c.Uint("p")
	privateKeyFile := c.String("w")

	if port == 0 {
		return nil, errors.New("port is missing")
	}

	if len(privateKeyFile) == 0 {
		return nil, errors.New("private key file is missing")
	}

	privateKeyBytes, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to open api private key file: %s", privateKeyFile)
	}

	client, err := wctl.NewClient(wctl.Config{
		APIHost:    host,
		APIPort:    port,
		PrivateKey: common.PrivateKey(privateKeyBytes),
		UseHTTPS:   false,
	})
	if err != nil {
		return nil, err
	}

	err = client.Init()
	if err != nil {
		return nil, err
	}

	return client, nil
}
