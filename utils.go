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
	"github.com/perlin-network/noise/skademlia"
	"math/rand"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

func SelectPeers(peers []*grpc.ClientConn, amount int) ([]*grpc.ClientConn, error) {
	if len(peers) < amount {
		return peers, errors.Errorf("only connected to %d peer(s), but require a minimum of %d peer(s)", len(peers), amount)
	}

	activePeers := make([]*grpc.ClientConn, 0, len(peers))
	for _, p := range peers {
		if p.GetState() == connectivity.Ready {
			activePeers = append(activePeers, p)
		}
	}

	if len(activePeers) > amount {
		rand.Shuffle(len(activePeers), func(i, j int) {
			activePeers[i], activePeers[j] = activePeers[j], activePeers[i]
		})

		activePeers = activePeers[:amount]
	}

	return activePeers, nil
}

type ClosestPeer struct {
	conn *grpc.ClientConn
	id   *skademlia.ID
}

// Return the list of closest peer's connection and it's ID.
func ClosestPeers(c *skademlia.Client, amount int, opts ...skademlia.DialOption) ([]ClosestPeer, error) {
	ids := c.ClosestPeerIDs()
	if len(ids) < amount {
		return nil, errors.Errorf("only connected to %d peer(s), but require a minimum of %d peer(s)", len(ids), amount)
	}

	var peers []ClosestPeer

	for i := range ids {
		if conn, err := c.Dial(ids[i].Address(), opts...); err == nil {
			peers = append(peers, ClosestPeer{
				conn: conn,
				id:   ids[i],
			})
		}
	}

	activePeers := make([]ClosestPeer, 0, len(peers))
	for _, p := range peers {
		if p.conn.GetState() == connectivity.Ready {
			activePeers = append(activePeers, p)
		}
	}

	if len(activePeers) > amount {
		rand.Shuffle(len(activePeers), func(i, j int) {
			activePeers[i], activePeers[j] = activePeers[j], activePeers[i]
		})

		activePeers = activePeers[:amount]
	}

	if len(activePeers) < amount {
		return nil, errors.Errorf("only connected to %d peer(s), but require a minimum of %d peer(s)", len(ids), amount)
	}

	return activePeers, nil
}
