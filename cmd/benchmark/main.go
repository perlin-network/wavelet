package main

import (
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/noise/edwards25519"
	logger "github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/valyala/fastjson"
	"gopkg.in/urfave/cli.v1"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	app := cli.NewApp()

	app.Name = "benchmark"
	app.Author = "Perlin"
	app.Email = "support@perlin.net"
	app.Version = sys.Version
	app.Usage = "a benchmarking tool for wavelet nodes"

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version:    %s\n", sys.Version)
		fmt.Printf("Go Version: %s\n", sys.GoVersion)
		fmt.Printf("Git Commit: %s\n", sys.GitCommit)
		fmt.Printf("OS/Arch:    %s\n", sys.OSArch)
		fmt.Printf("Built:      %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	app.Before = func(context *cli.Context) error {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger().Output(logger.NewConsoleWriter())

		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:  "local",
			Usage: "spawn some number of nodes locally and spam transactions on all of them",
			Flags: []cli.Flag{
				cli.UintFlag{
					Name:  "count",
					Usage: "number of nodes to spawn",
					Value: 2,
				},
			},
			Action: commandLocal,
		},
		{
			Name:  "remote",
			Usage: "connect to an already-running node and spam transactions on it",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "host",
					Usage: "node HTTP API address",
					Value: "127.0.0.1:9000",
				},
				cli.StringFlag{
					Name:  "sk",
					Usage: "private key in hex format to connect to node HTTP API with",
					Value: "87a6813c3b4cf534b6ae82db9b1409fa7dbd5c13dba5858970b56084c4a930eb400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
				},
			},
			Action: commandRemote,
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(os.Args); err != nil {
		fmt.Printf("failed to parse configuration/command-line arguments: %+v\n", err)
	}
}

func commandRemote(c *cli.Context) error {
	args := strings.Split(c.String("host"), ":")

	if len(args) != 2 || len(args[0]) == 0 || len(args[1]) == 0 {
		return errors.New("host and port must be specified [example: 127.0.0.1:3000]")
	}

	host := args[0]

	var port uint16

	if p, err := strconv.ParseUint(args[1], 10, 16); err != nil {
		return errors.Wrap(err, "failed to decode port")
	} else {
		port = uint16(p)
	}

	privateKeyHex := c.String("sk")

	if len(privateKeyHex) != edwards25519.SizePrivateKey*2 {
		return errors.New("private key size is invalid")
	}

	privateKeyBuf, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return errors.Wrap(err, "failed to decode private key")
	}

	var privateKey edwards25519.PrivateKey
	copy(privateKey[:], privateKeyBuf)

	client, err := connectToAPI(host, port, privateKey)
	if err != nil {
		return errors.Wrap(err, "failed to connect to node HTTP API")
	}

	fmt.Println("You're now connected!")

	go func() {
		events, err := client.PollLoggerSink(nil, wctl.RouteWSMetrics)
		if err != nil {
			panic(err)
		}

		var p fastjson.Parser

		for evt := range events {
			v, err := p.ParseBytes(evt)

			if err != nil {
				continue
			}

			metrics := v.Get("metrics")

			log.Info().
				Float64("accepted_tps", metrics.GetFloat64("tx.accepted", "mean.rate")).
				Float64("received_tps", metrics.GetFloat64("tx.received", "mean.rate")).
				Msg("Benchmarking...")
		}
	}()

	flood := floodTransactions()

	for {
		if _, err := flood(client); err != nil {
			fmt.Println(err)
		}
	}
}

func commandLocal(c *cli.Context) error {
	build()

	count := c.Uint("count")

	if count == 0 {
		return errors.New("count must be > 0")
	}

	nodes := []*node{
		spawn(nextAvailablePort(), nextAvailablePort(), false),
	}

	for i := uint(0); i < count-1; i++ {
		nodes = append(nodes, spawn(nextAvailablePort(), nextAvailablePort(), true, fmt.Sprintf("127.0.0.1:%d", nodes[0].nodePort)))
	}

	wait(nodes...)

	fmt.Println("Nodes are initialized!")

	flood := floodTransactions()

	for {
		if _, err := flood(nodes[0].client); err != nil {
			fmt.Println(err)
		}
	}
}
