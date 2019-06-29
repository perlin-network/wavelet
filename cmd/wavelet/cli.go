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
	c := &CLI{
		client: client,
		ledger: ledger,
		logger: log.Node(),
		keys:   keys,
		app:    cli.NewApp(),
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
			Name:    "exit",
			Aliases: []string{"quit", ":q"},
			Action:  a(c.exit),
		},
	}

	// Generate the help message
	s := strings.Builder{}
	s.WriteString("Commands:\n")
	w := tabwriter.NewWriter(&s, 0, 0, 1, ' ', 0)

	for _, c := range c.app.VisibleCommands() {
		fmt.Fprintf(w,
			"    %s (%s) %s\t%s\n",
			c.Name, strings.Join(c.Aliases, ", "), c.Usage,
			c.Description,
		)
	}

	w.Flush()
	c.app.CustomAppHelpTemplate = s.String()

	// Add in autocompletion
	var completers = make(
		[]readline.PrefixCompleterInterface,
		0, len(c.app.Commands)*2+1,
	)

	for _, cmd := range c.app.Commands {
		completers = append(completers, readline.PcItem(
			cmd.Name, c.getCompleter(),
		))

		for _, alias := range cmd.Aliases {
			completers = append(completers, readline.PcItem(
				alias, c.getCompleter(),
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

	c.rl = rl

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

	return c, nil
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
