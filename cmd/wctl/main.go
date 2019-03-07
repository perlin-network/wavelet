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

				tx, err := client.GetTxnByID(txID)
				if err != nil {
					return err
				}

				jsonOut, _ := json.Marshal(tx)
				fmt.Printf("%s\n", jsonOut)
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

	config := wctl.Config{
		APIHost:  host,
		APIPort:  uint16(port),
		UseHTTPS: false,
	}
	copy(config.RawPrivateKey[:], privateKeyBytes)
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
