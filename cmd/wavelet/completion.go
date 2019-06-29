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

import "github.com/chzyer/readline"

/*
func (cli *CLI) getCompletionPeers() *readline.PrefixCompleter {
	fn := func(s string) (l []string) {
		// Get a list of peers
		peers := cli.client.ClosestPeerIDs()
		l = make([]string, 0, len(peers))

		for _, id := range peers {
			pub := id.PublicKey()
			l = append(l, hex.EncodeToString(pub[:]))
		}

		// Get current ID
		publicKey := cli.keys.PublicKey()

		// Get root ID
		round := cli.ledger.Rounds().Latest()

		l = append(l,
			hex.EncodeToString(round.End.ID[:]),
			hex.EncodeToString(publicKey[:]),
		)

		fields := strings.Fields(s)

		if len(fields) > 1 {
			l = containStrings(l, fields[1], false)
		}

		return
	}

	return readline.PcItemDynamic(fn)
}
*/

func (cli *CLI) getCompleter() *readline.PrefixCompleter {
	return readline.PcItemDynamic(func(string) []string {
		return cli.completion
	})
}

func (cli *CLI) addCompletion(ids ...string) {
MainLoop:
	for _, id := range ids {
		for _, c := range cli.completion {
			if c == id {
				continue MainLoop
			}
		}

		cli.completion = append(cli.completion, id)
	}
}
