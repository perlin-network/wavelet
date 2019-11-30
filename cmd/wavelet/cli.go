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
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"gopkg.in/urfave/cli.v1"

	"github.com/benpye/readline"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/rs/zerolog"
)

const (
	vtRed   = "\033[31m"
	vtReset = "\033[39m"
	prompt  = "»»»"
)

type CLI struct {
	app    *cli.App
	rl     *readline.Instance
	logger *zerolog.Logger

	client *wctl.Client

	stdin  io.ReadCloser
	stdout io.Writer

	cleanup func()
}

func CLIWithStdin(stdin io.ReadCloser) func(cli *CLI) {
	return func(cli *CLI) {
		cli.stdin = stdin
	}
}

func CLIWithStdout(stdout io.Writer) func(cli *CLI) {
	return func(cli *CLI) {
		cli.stdout = stdout
	}
}

func NewCLI(client *wctl.Client, opts ...func(cli *CLI)) (*CLI, error) {
	// Set CLI callbacks, mainly loggers
	cleanup, err := setEvents(client)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to start websockets to the server: %v", err)
	}

	c := &CLI{
		client:  client,
		logger:  log.Node(),
		app:     cli.NewApp(),
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		cleanup: cleanup,
	}

	for _, o := range opts {
		o(c)
	}

	c.app.Name = "wavelet"
	c.app.HideVersion = true
	c.app.UsageText = "command [arguments...]"
	c.app.CommandNotFound = func(ctx *cli.Context, s string) {
		c.logger.Error().
			Msg("Unknown command: " + s)
	}

	// List of commands and their actions
	c.app.Commands = []cli.Command{
		{
			Name:        "status",
			Aliases:     []string{"l"},
			Action:      a(c.status),
			Description: "print out information about your node",
		},
		{
			Name:        "pay",
			Aliases:     []string{"p"},
			Action:      a(c.pay),
			Description: "pay the address an amount of PERLs",
		},
		{
			Name:        "call",
			Aliases:     []string{"c"},
			Action:      a(c.call),
			Description: "invoke a function on a smart contract",
		},
		{
			Name:        "find",
			Aliases:     []string{"f"},
			Action:      a(c.find),
			Description: "search for any wallet/smart contract/transaction",
		},
		{
			Name:        "spawn",
			Aliases:     []string{"s"},
			Action:      a(c.spawn),
			Description: "test deploy a smart contract",
		},
		{
			Name:        "deposit-gas",
			Aliases:     []string{"g"},
			Action:      a(c.depositGas),
			Description: "deposit gas to a smart contract",
		},
		{
			Name:        "place-stake",
			Aliases:     []string{"ps"},
			Action:      a(c.placeStake),
			Description: "deposit a stake of PERLs into the network",
		},
		{
			Name:        "withdraw-stake",
			Aliases:     []string{"ws"},
			Action:      a(c.withdrawStake),
			Description: "withdraw stake and diminish voting power",
		},
		{
			Name:        "withdraw-reward",
			Aliases:     []string{"wr"},
			Action:      a(c.withdrawReward),
			Description: "withdraw rewards into PERLs",
		},
		{
			Name:        "connect",
			Aliases:     []string{"cc"},
			Action:      a(c.connect),
			Description: "connect to a peer",
		},
		{
			Name:        "disconnect",
			Aliases:     []string{"dc"},
			Action:      a(c.disconnect),
			Description: "disconnect a peer",
		},
		{
			Name:        "restart",
			Aliases:     []string{"r"},
			Action:      a(c.restart),
			Description: "restart node",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "hard",
					Usage: "database will be erased if provided",
				},
			},
		},
		{
			Name:    "exit",
			Aliases: []string{"quit", ":q"},
			Action:  a(c.exit),
		},
		{
			Name:      "update-params",
			UsageText: "Updates parameters, if no value provided, default one will be used.",
			Aliases:   []string{"up"},
			Action:    a(c.updateParameters),
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "snowball.k",
					Value: conf.GetSnowballK(),
					Usage: "snowball K consensus parameter",
				},
				cli.IntFlag{
					Name:  "snowball.beta",
					Value: conf.GetSnowballBeta(),
					Usage: "snowball Beta consensus parameter",
				},
				cli.Float64Flag{
					Name:  "vote.sync.threshold",
					Value: conf.GetSyncVoteThreshold(),
					Usage: "threshold used to determine majority for sync vote counting",
				},
				cli.Float64Flag{
					Name:  "vote.finalization.threshold",
					Value: conf.GetFinalizationVoteThreshold(),
					Usage: "threshold used to determine majority for finalization vote counting",
				},
				cli.Float64Flag{
					Name:  "vote.finalization.stake.weight",
					Value: conf.GetStakeMajorityWeight(),
					Usage: "weight for stake percentage used in finalization majority calculation ",
				},
				cli.Float64Flag{
					Name:  "vote.finalization.transactions.weight",
					Value: conf.GetTransactionsNumMajorityWeight(),
					Usage: "weight for percentage of max transactions number used in finalization majority calculation",
				},
				cli.DurationFlag{
					Name:  "query.timeout",
					Value: conf.GetQueryTimeout(),
					Usage: "timeout for query request",
				},
				cli.DurationFlag{
					Name:  "gossip.timeout",
					Value: conf.GetGossipTimeout(),
					Usage: "timeout for gossip request",
				},
				cli.DurationFlag{
					Name:  "download.tx.timeout",
					Value: conf.GetDownloadTxTimeout(),
					Usage: "timeout for download tx request",
				},
				cli.DurationFlag{
					Name:  "check.out.of.sync.timeout",
					Value: conf.GetCheckOutOfSyncTimeout(),
					Usage: "timeout for check out of sync request",
				},
				cli.IntFlag{
					Name:  "sync.chunk.size",
					Value: conf.GetSyncChunkSize(),
					Usage: "chunk size for state syncing",
				},
				cli.Uint64Flag{
					Name:  "sync.if.block.indices.differ.by",
					Value: conf.GetSyncIfBlockIndicesDifferBy(),
					Usage: "difference in blocks between nodes which initiates state syncing",
				},
				cli.Uint64Flag{
					Name:  "pruning.limit",
					Value: uint64(conf.GetPruningLimit()),
					Usage: "number of blocks after which pruning of transactions will happen",
				},
				cli.StringFlag{
					Name:  "api.secret",
					Value: conf.GetSecret(),
					Usage: "shared secret for http api authorization",
				},
				cli.UintFlag{
					Name:  "bloom.filter.k",
					Value: conf.GetBloomFilterK(),
					Usage: "bloom filter K parameter for transaction syncing",
				},
				cli.UintFlag{
					Name:  "bloom.filter.m",
					Value: conf.GetBloomFilterM(),
					Usage: "bloom filter M parameter for transaction syncing",
				},
				cli.Uint64Flag{
					Name:  "tx.sync.chunk.size",
					Value: conf.GetTXSyncChunkSize(),
					Usage: "number of transactions per chunk for transaction syncing",
				},
				cli.Uint64Flag{
					Name:  "tx.sync.limit",
					Value: conf.GetTXSyncLimit(),
					Usage: "max number of transactions to be synced",
				},
			},
		},
		{
			Name:        "dump",
			Action:      a(c.dump),
			Description: "dump wallet states, and you may use -c to dump contract code and pages",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "c",
					Usage: "dump contract code and pages",
				},
			},
		},
		{
			Name:        "dump",
			Action:      a(c.dump),
			Description: "dump wallet states, and you may use -c to dump contract code and pages",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "c",
					Usage: "dump contract code and pages",
				},
			},
		},
	}

	// Generate the help message
	s := strings.Builder{}
	s.WriteString("Commands:\n")
	w := tabwriter.NewWriter(&s, 0, 0, 1, ' ', 0)

	for _, c := range c.app.VisibleCommands() {
		_, err := fmt.Fprintf(w,
			"    %s (%s) %s\t%s\n",
			c.Name, strings.Join(c.Aliases, ", "), c.Usage,
			c.Description,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := w.Flush(); err != nil {
		return nil, err
	}

	_ = w.Flush()
	c.app.CustomAppHelpTemplate = s.String()

	// Add in autocompletion
	var completers = make(
		[]readline.PrefixCompleterInterface,
		0, len(c.app.Commands)*2+1,
	)

	for _, cmd := range c.app.Commands {
		switch cmd.Name {
		case "spawn":
			commandAddCompleter(&completers, cmd,
				c.getPathCompleter())
		default:
			commandAddCompleter(&completers, cmd,
				c.getCompleter())
		}
	}

	var completer = readline.NewPrefixCompleter(completers...)

	// Make a new readline struct
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            vtRed + prompt + vtReset + " ",
		AutoComplete:      completer,
		HistoryFile:       "/tmp/wavelet-history.tmp",
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
		Stdin:             c.stdin,
		Stdout:            c.stdout,
	})

	if err != nil {
		return nil, err
	}

	c.rl = rl

	log.SetWriter(
		log.LoggerWavelet,
		log.NewConsoleWriter(
			rl.Stdout(),
			log.FilterFor(
				log.ModuleNode,
				log.ModuleSync,
				log.ModuleContract,
			),
		),
	)

	return c, nil
}

func (cli *CLI) Start() {
	defer cli.cleanup()

ReadLoop:
	for {
		line, err := cli.rl.Readline()
		switch err {
		case readline.ErrInterrupt:
			if len(line) == 0 {
				break ReadLoop
			}

			continue ReadLoop

		case io.EOF:
			break ReadLoop
		}

		r := csv.NewReader(strings.NewReader(line))
		r.Comma = ' '

		s, err := r.Read()
		if err != nil {
			s = strings.Fields(line)
		}

		// Add an app name as $0
		s = append([]string{cli.app.Name}, s...)

		if err := cli.app.Run(s); err != nil {
			cli.logger.Error().Err(err).
				Msg("Failed to run command.")
		}
	}

	_ = cli.rl.Close()
}

func (cli *CLI) exit(ctx *cli.Context) {
	_ = cli.rl.Close()
}

func a(f func(*cli.Context)) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		f(ctx)
		return nil
	}
}

func (cli *CLI) parseRecipient(arg string) ([32]byte, bool) {
	var recipient [32]byte

	i, err := hex.Decode(recipient[:], []byte(arg))
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("The ID you specified is invalid.")
		return recipient, false
	}

	if i != 32 {
		cli.logger.Error().Int("length", len(recipient)).
			Msg("The ID you specified is invalid.")
		return recipient, false
	}

	return recipient, true
}

func (cli *CLI) parseAmount(arg string) (a uint64, ok bool) {
	amount, err := strconv.ParseUint(arg, 10, 64)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to convert payment amount to a uint64.")
		return 0, false
	}

	return amount, true
}
