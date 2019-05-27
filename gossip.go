package wavelet

import (
	"context"
	"github.com/perlin-network/noise/skademlia"
	"sync"
)

type Gossiper struct {
	client  *skademlia.Client
	metrics *Metrics

	streams     map[string]Wavelet_GossipClient
	streamsLock sync.Mutex

	debouncer *TransactionDebouncer
}

func NewGossiper(ctx context.Context, client *skademlia.Client, metrics *Metrics, opts ...TransactionDebouncerOption) *Gossiper {
	g := &Gossiper{
		client:  client,
		metrics: metrics,

		streams: make(map[string]Wavelet_GossipClient),
	}

	g.debouncer = NewTransactionDebouncer(ctx, append(opts, WithAction(g.Gossip))...)

	return g
}

func (g *Gossiper) Push(tx Transaction) {
	g.debouncer.Push(tx)

	if g.metrics != nil {
		g.metrics.gossipedTX.Mark(int64(tx.LogicalUnits()))
	}
}

func (g *Gossiper) Gossip(transactions [][]byte) {
	var err error

	batch := &Transactions{Transactions: transactions}

	conns := g.client.AllPeers()

	var wg sync.WaitGroup
	wg.Add(len(conns))

	for _, conn := range conns {
		target := conn.Target()

		g.streamsLock.Lock()
		stream, exists := g.streams[conn.Target()]

		if !exists {
			client := NewWaveletClient(conn)

			if stream, err = client.Gossip(context.Background()); err != nil {
				g.streamsLock.Unlock()
				continue
			}

			g.streams[target] = stream
		}
		g.streamsLock.Unlock()

		go func() {
			if err := stream.Send(batch); err != nil {
				g.streamsLock.Lock()
				delete(g.streams, target)
				g.streamsLock.Unlock()
			}

			wg.Done()
		}()
	}

	wg.Wait()
}
