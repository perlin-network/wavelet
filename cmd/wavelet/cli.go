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
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/chzyer/readline"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
	"github.com/urfave/cli"
)

const (
	vtRed   = "\033[31m"
	vtReset = "\033[39m"
	prompt  = "»»»"
)

type CLI struct {
	app    *cli.App
	rl     *readline.Instance
	client *skademlia.Client
	ledger *wavelet.Ledger
	logger zerolog.Logger
	keys   *skademlia.Keypair

	completion []string
}

func NewCLI(client *skademlia.Client, ledger *wavelet.Ledger, keys *skademlia.Keypair) (*CLI, error) {
	CLI := &CLI{
		client: client,
		ledger: ledger,
		logger: log.Node(),
		keys:   keys,
		app:    cli.NewApp(),
	}

	CLI.app.Name = "wavelet"
	CLI.app.HideVersion = true
	CLI.app.UsageText = "command [arguments...]"
	CLI.app.CommandNotFound = func(ctx *cli.Context, s string) {
		CLI.logger.Error().
			Msg("Unknown command: " + s)
	}

	// List of commands and their actions
	CLI.app.Commands = []cli.Command{
		{
			Name:        "status",
			Aliases:     []string{"l"},
			Action:      a(CLI.status),
			Description: "print out information about your node",
		},
		{
			Name:        "pay",
			Aliases:     []string{"p"},
			Action:      a(CLI.pay),
			Description: "pay the address an amount of PERLs",
		},
		{
			Name:        "call",
			Aliases:     []string{"c"},
			Action:      a(CLI.call),
			Description: "invoke a function on a smart contract",
		},
		{
			Name:        "find",
			Aliases:     []string{"f"},
			Action:      a(CLI.find),
			Description: "search for any wallet/smart contract/transaction",
		},
		{
			Name:        "spawn",
			Aliases:     []string{"s"},
			Action:      a(CLI.spawn),
			Description: "test deploy a smart contract",
		},
		{
			Name:        "place-stake",
			Aliases:     []string{"ps"},
			Action:      a(CLI.placeStake),
			Description: "deposit a stake of PERLs into the network",
		},
		{
			Name:        "withdraw-stake",
			Aliases:     []string{"ws"},
			Action:      a(CLI.withdrawStake),
			Description: "withdraw stake and diminish voting power",
		},
		{
			Name:        "withdraw-reward",
			Aliases:     []string{"wr"},
			Action:      a(CLI.withdrawReward),
			Description: "withdraw rewards into PERLs",
		},
		{
			Name:    "exit",
			Aliases: []string{"quit", ":q"},
			Action:  a(CLI.exit),
		},
	}

	// Generate the help message
	s := strings.Builder{}
	s.WriteString("Commands:\n")
	w := tabwriter.NewWriter(&s, 0, 0, 1, ' ', 0)

	for _, c := range CLI.app.VisibleCommands() {
		fmt.Fprintf(w,
			"    %s (%s) %s\t%s\n",
			c.Name, strings.Join(c.Aliases, ", "), c.Usage,
			c.Description,
		)
	}

	w.Flush()
	CLI.app.CustomAppHelpTemplate = s.String()

	// Add in autocompletion
	var completers = make(
		[]readline.PrefixCompleterInterface,
		0, len(CLI.app.Commands)*2+1,
	)

	for _, cmd := range CLI.app.Commands {
		completers = append(completers, readline.PcItem(
			cmd.Name, CLI.getCompleter(),
		))

		for _, alias := range cmd.Aliases {
			completers = append(completers, readline.PcItem(
				alias, CLI.getCompleter(),
			))
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
	})

	if err != nil {
		return nil, err
	}

	CLI.rl = rl

	log.SetWriter(
		log.LoggerWavelet,
		log.NewConsoleWriter(rl.Stdout(), log.FilterFor(
			log.ModuleNode,
			log.ModuleNetwork,
			log.ModuleSync,
			log.ModuleConsensus,
			log.ModuleContract,
		)),
	)

	return CLI, nil
}

func (cli *CLI) Start() {
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

	cli.rl.Close()
}

func (cli *CLI) exit(ctx *cli.Context) {
	cli.rl.Close()
}

func a(f func(*cli.Context)) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		f(ctx)
		return nil
	}
}
