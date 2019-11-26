// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	logger "github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/urfave/cli.v1"
)

func main() {
	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100

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
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger().Output(logger.NewConsoleWriter(nil))

		return nil
	}

	app.Commands = []cli.Command{
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
					Name:  "wallet",
					Usage: "private key in hex format to connect to node HTTP API with",
					Value: "87a6813c3b4cf534b6ae82db9b1409fa7dbd5c13dba5858970b56084c4a930eb400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405", // nolint:lll
				},
				cli.IntFlag{
					Name: "worker, w",
					Usage: "the number of workers used to spam transactions per iteration, where one worker will send" +
						" one transaction. default to the number of logical CPU and min is 1.",
					Value: runtime.NumCPU(),
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

func getKeys(wallet string) (edwards25519.PrivateKey, edwards25519.PublicKey) {
	var (
		emptyPrivateKey edwards25519.PrivateKey
		emptyPublicKey  edwards25519.PublicKey
	)

	privateKeyBuf, err := ioutil.ReadFile(wallet)
	if err != nil {
		if os.IsNotExist(err) {
			// If a private key is specified instead of a path to a wallet, then simply use the provided private key instead.
			if len(wallet) == hex.EncodedLen(edwards25519.SizePrivateKey) {
				var privateKey edwards25519.PrivateKey

				n, err := hex.Decode(privateKey[:], []byte(wallet))
				if err != nil {
					log.Fatal().Err(err).Msgf("Failed to decode the private key specified: %s", wallet)
				}

				if n != edwards25519.SizePrivateKey {
					log.Fatal().Msgf("Private key %s is not of the right length.", wallet)
					return emptyPrivateKey, emptyPublicKey
				}

				k, err := skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
				if err != nil {
					log.Fatal().Err(err).Msgf("The private key specified is invalid: %s", wallet)
					return emptyPrivateKey, emptyPublicKey
				}

				return k.PrivateKey(), k.PublicKey()
			}

			log.Fatal().Msgf("Could not find an existing wallet at %q.", wallet)

			return emptyPrivateKey, emptyPublicKey
		}

		log.Warn().Err(err).Msgf("Encountered an unexpected error loading your wallet from %q.", wallet)

		return emptyPrivateKey, emptyPublicKey
	}

	var privateKey edwards25519.PrivateKey

	n, err := hex.Decode(privateKey[:], privateKeyBuf)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to decode your private key from %q.", wallet)
		return emptyPrivateKey, emptyPublicKey
	}

	if n != edwards25519.SizePrivateKey {
		log.Fatal().Msgf("Private key located in %q is not of the right length.", wallet)
		return emptyPrivateKey, emptyPublicKey
	}

	k, err := skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		log.Fatal().Err(err).Msgf("The private key specified in %q is invalid.", wallet)
		return emptyPrivateKey, emptyPublicKey
	}

	return k.PrivateKey(), k.PublicKey()
}

func commandRemote(c *cli.Context) error {
	args := strings.Split(c.String("host"), ":")

	if len(args) != 2 || len(args[0]) == 0 || len(args[1]) == 0 {
		return errors.New("host and port must be specified [example: 127.0.0.1:3000]")
	}

	host := args[0]

	var port uint16

	p, err := strconv.ParseUint(args[1], 10, 16)
	if err != nil {
		return errors.Wrap(err, "failed to decode port")
	}

	port = uint16(p)

	wallet := c.String("wallet")

	privateKey, publicKey := getKeys(wallet)

	log.Info().
		Hex("privateKey", privateKey[:]).
		Hex("publicKey", publicKey[:]).
		Msg("Loaded wallet.")

	numWorkers := c.Int("worker")
	if numWorkers < 0 {
		numWorkers = 1
	}

	client, err := connectToAPI(host, port, privateKey)
	if err != nil {
		return errors.Wrap(err, "failed to connect to node HTTP API")
	}

	defer client.Close()

	fmt.Printf("You're now connected! Using %d workers\n.", numWorkers)

	// Add the OnMetrics callback
	client.OnMetrics = func(met wctl.Metrics) {
		log.Info().
			Float64("accepted_tps", met.TpsAccepted).
			Float64("received_tps", met.TpsReceived).
			Float64("gossiped_tps", met.TpsGossiped).
			Float64("downloaded_tps", met.TpsDownloaded).
			Float64("queried_bps", met.BpsQueried).
			Int64("query_latency_max_ms", met.QueryLatencyMaxMS).
			Int64("query_latency_min_ms", met.QueryLatencyMinMS).
			Float64("query_latency_mean_ms", met.QueryLatencyMeanMS).
			Str("message", met.Message).
			Msg("Benchmarking...")
	}

	if _, err := client.PollMetrics(); err != nil {
		panic(err)
	}

	flood := floodTransactions(numWorkers)

	for {
		if _, err := flood(client); err != nil {
			continue
		}
	}
}
