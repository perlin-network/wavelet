package node

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"math/rand"
	"sync"
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

type broadcastPayload struct {
	order int
	res   chan struct{}
	peer  *grpc.ClientConn
	fn    func(int, WaveletClient) error
}

type broadcaster struct {
	bus chan broadcastPayload

	wg     sync.WaitGroup
	cancel func()
}

func NewBroadcaster(workersNum int, capacity uint32) *broadcaster {
	ctx, cancel := context.WithCancel(context.Background())
	b := broadcaster{
		bus:    make(chan broadcastPayload, capacity),
		cancel: cancel,
	}

	b.wg.Add(workersNum)

	for i := 0; i < workersNum; i++ {
		go func(ctx context.Context) {
			defer b.wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case payload := <-b.bus:
					if err := payload.fn(payload.order, NewWaveletClient(payload.peer)); err != nil {
						fmt.Println("error broadcasting:", err)
					}

					payload.res <- struct{}{}
				}
			}
		}(ctx)
	}

	return &b
}

func (b *broadcaster) Broadcast(ctx context.Context, peers []*grpc.ClientConn, fn func(int, WaveletClient) error) {
	resChan := make(chan struct{}, len(peers))
	defer close(resChan)

	for i, peer := range peers {
		b.bus <- broadcastPayload{
			order: i,
			peer:  peer,
			res:   resChan,
			fn:    fn,
		}
	}

	for i := 0; i < len(peers); i++ {
		select {
		case <-ctx.Done():
			return
		case <-resChan:
		}
	}

	return
}

func (b *broadcaster) Stop() {
	b.cancel()
	b.wg.Wait()

	close(b.bus)
}
