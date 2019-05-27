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
