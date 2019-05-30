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

package wavelet

import (
	"encoding/hex"
	"fmt"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"io/ioutil"
	"math/rand"
	"strconv"
	"strings"
)

func SelectPeers(peers []*grpc.ClientConn, amount int) ([]*grpc.ClientConn, error) {
	if len(peers) < amount {
		return peers, errors.Errorf("only connected to %d peer(s), but require a minimum of %d peer(s)", len(peers), amount)
	}

	if len(peers) > amount {
		rand.Shuffle(len(peers), func(i, j int) {
			peers[i], peers[j] = peers[j], peers[i]
		})

		peers = peers[:amount]
	}

	return peers, nil
}

func ExportGraphDOT(round *Round, graph *Graph) {
	visited := map[TransactionID]struct{}{round.Start.ID: {}}

	queue := AcquireQueue()
	defer ReleaseQueue(queue)

	queue.PushBack(&round.End)

	var dot strings.Builder

	dot.WriteString("digraph G {")

	for queue.Len() > 0 {
		popped := queue.PopFront().(*Transaction)

		dot.WriteByte('\n')
		dot.WriteByte('\t')
		dot.WriteString(strconv.Quote(hex.EncodeToString(popped.ID[:])))
		dot.WriteString(" -> ")
		dot.WriteByte('{')
		dot.WriteByte(' ')

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; seen {
				continue
			}

			visited[parentID] = struct{}{}

			parent := graph.FindTransaction(parentID)

			if parent == nil {
				return
			}

			if parent.Depth <= round.Start.Depth {
				continue
			}

			queue.PushBack(parent)

			dot.WriteString(strconv.Quote(hex.EncodeToString(parentID[:])))
			dot.WriteByte(' ')
		}

		dot.WriteByte('}')
	}

	dot.WriteByte('\n')
	dot.WriteByte('}')

	if err := ioutil.WriteFile(fmt.Sprintf("rounds/%d.dot", round.Index), []byte(dot.String()), 0644); err != nil {
		fmt.Println("error saving graph:", err)
	}
}
