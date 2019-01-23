package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"time"

	"github.com/perlin-network/graph/wire"
	apiClient "github.com/perlin-network/wavelet/cmd/wctl/client"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/params"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func main() {

	app := cli.NewApp()

	app.Name = "wctl"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = params.Version
	app.Usage = "a cli client to interact with the wavelet node"

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version:    %s\n", c.App.Version)
		fmt.Printf("Go Version: %s\n", params.GoVersion)
		fmt.Printf("Git Commit: %s\n", params.GitCommit)
		fmt.Printf("OS/Arch:    %s\n", params.OSArch)
		fmt.Printf("Built:      %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	commonFlags := []cli.Flag{
		cli.StringFlag{
			Name:  "api.host",
			Value: "localhost",
			Usage: "Host of the local HTTP API `API_HOST`.",
		},
		cli.IntFlag{
			Name:  "api.port",
			Usage: "Port of the local HTTP API `API_PORT` (required).",
		},
		cli.StringFlag{
			Name:  "api.private_key_file",
			Usage: "The file containing private key that will make transactions through the API `API_PRIVATE_KEY_FILE` (required).",
		},
		cli.StringFlag{
			Name:  "log_level",
			Value: "info",
			Usage: "Minimum level at which logs will be printed to stdout. One of off|debug|info|warn|error|fatal `LOG_LEVEL`.",
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
				tag := c.Args().Get(0)
				payload := c.Args().Get(1)
				tx, err := client.SendTransaction(tag, []byte(payload))
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
				transactions, err := client.RecentTransactions("")
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
					log.Info().Msgf("%v", ev)
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
				log.Info().Msgf("%s", string(jsonOut))
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
				log.Info().Msgf("saved contract %s to file %s", contractID, filename)
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
		log.Error().Msgf("Failed to parse configuration/command-line arguments: %v", err)
	}
}

func setup(c *cli.Context) (*apiClient.Client, error) {
	host := c.String("api.host")
	port := c.Uint("api.port")
	privateKeyFile := c.String("api.private_key_file")
	log.SetLevel(c.String("log_level"))

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

	client, err := apiClient.NewClient(apiClient.Config{
		APIHost:    host,
		APIPort:    port,
		PrivateKey: string(privateKeyBytes),
		UseHTTPS:   false,
	})
	if err != nil {
		return nil, err
	}

	err = client.Init()
	if err != nil {
		return nil, err
	}

	log.Debug().Str("SessionToken", client.SessionToken).Msg(" ")
	return client, nil
}
