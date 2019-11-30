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
	"math/rand"

	"github.com/perlin-network/noise/skademlia"

	"github.com/pkg/errors"
	"google.golang.org/grpc/connectivity"
)

func SelectPeers(peers []skademlia.ClosestPeer, amount int) ([]skademlia.ClosestPeer, error) {
	if len(peers) < amount {
		return peers, errors.Errorf("only connected to %d peer(s), but require a minimum of %d peer(s)", len(peers), amount)
	}

	activePeers := make([]skademlia.ClosestPeer, 0, len(peers))

	for _, p := range peers {
		if p.Conn().GetState() == connectivity.Ready {
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
