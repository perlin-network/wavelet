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
	"os"
	"path/filepath"
	"strings"

	"github.com/benpye/readline"
	"github.com/urfave/cli"
)

func (cli *CLI) getCompleter() *readline.PrefixCompleter {
	if cli.client.Server == nil {
		return nil
	}

	return readline.PcItemDynamic(func(line string) []string {
		f := strings.Split(line, " ")
		if len(f) < 2 {
			return nil
		}

		text := f[len(f)-1]

		return cli.client.Server.Ledger.Find(text, 10)
	})
}

type PathCompleter struct {
	*readline.PrefixCompleter
}

func (p *PathCompleter) GetDynamicNames(line []rune) [][]rune {
	var path string
	words := strings.Split(string(line), " ")
	if len(words) > 1 && words[1] != "" { // has some file
		path = filepath.Dir(strings.Join(words[1:], " "))
	} else {
		path = "."
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}

	defer f.Close()

	files, err := f.Readdir(-1)
	if err != nil {
		return nil
	}

	names := make([][]rune, 0, len(files))

	for _, f := range files {
		filename := filepath.Join(path, f.Name())
		if f.IsDir() {
			filename += "/"
		} else {
			filename += " "
		}

		names = append(names, []rune(filename))
	}

	return names
}

func (cli *CLI) getPathCompleter() readline.PrefixCompleterInterface {
	return &PathCompleter{
		PrefixCompleter: &readline.PrefixCompleter{
			Callback: func(string) []string { return nil },
			Dynamic:  true,
			Children: nil,
		},
	}
}

func commandAddCompleter(completers *[]readline.PrefixCompleterInterface,
	cmd cli.Command, completer readline.PrefixCompleterInterface) {

	*completers = append(*completers, readline.PcItem(
		cmd.Name, completer,
	))

	for _, alias := range cmd.Aliases {
		*completers = append(*completers, readline.PcItem(
			alias, completer,
		))
	}
}
