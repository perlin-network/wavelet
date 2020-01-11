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
	"context"
	"sync"
	"time"

	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/internal/debounce"
	"github.com/perlin-network/wavelet/log"
)

type Gossiper struct {
	client  *skademlia.Client
	metrics *Metrics

	debouncer *debounce.Limiter
}

func NewGossiper(ctx context.Context, client *skademlia.Client, metrics *Metrics) *Gossiper {
	g := &Gossiper{
		client:  client,
		metrics: metrics,
	}

	g.debouncer = debounce.NewLimiter(
		ctx,
		debounce.WithAction(g.Gossip),
		debounce.WithPeriod(100*time.Millisecond),
		debounce.WithBufferLimit(16384),
	)

	return g
}

func (g *Gossiper) Push(tx Transaction) {
	g.debouncer.Add(debounce.Bytes(tx.Marshal()))

	if g.metrics != nil {
		g.metrics.gossipedTX.Mark(int64(tx.LogicalUnits()))
	}
}

func (g *Gossiper) Gossip(transactions [][]byte) {
	logger := log.TX("gossip")

	peers, err := SelectPeers(g.client.ClosestPeers(), conf.GetSnowballK())
	if err != nil {
		logger.Err(err).Msg("Failed to select peers to gossip transactions.")
		return
	}

	batch := &GossipRequest{Transactions: transactions}

	var wg sync.WaitGroup

	for _, p := range peers {
		wg.Add(1)

		go func(p skademlia.ClosestPeer) {
			defer wg.Done()

			client := NewWaveletClient(p.Conn())

			ctx, cancel := context.WithTimeout(context.Background(), conf.GetGossipTimeout())
			defer cancel()

			_, err := client.Gossip(ctx, batch)
			if err != nil {
				logger.Err(err).Msg("Failed to send batch")
			}
		}(p)
	}

	wg.Wait()
}
