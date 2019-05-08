package node

import (
	"context"
	"github.com/perlin-network/noise"
	"github.com/pkg/errors"
	"math/rand"
	"sync"
)

func SelectPeers(peers []*noise.Peer, amount int) ([]*noise.Peer, error) {
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

type broadcastResponse struct {
	body  []byte
	order int
}

type broadcastPayload struct {
	order           int
	requestOpcode   byte
	res             chan broadcastResponse
	waitForResponse bool
	body            []byte
	peer            *noise.Peer
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
					func() {
						res := broadcastResponse{
							order: payload.order,
						}

						defer func() {
							payload.res <- res
						}()

						if !payload.waitForResponse {
							_ = payload.peer.Send(payload.requestOpcode, payload.body)
							return
						}

						res.body, _ = payload.peer.Request(payload.requestOpcode, payload.body)
					}()
				}
			}
		}(ctx)
	}

	return &b
}

func (b *broadcaster) Broadcast(ctx context.Context, peers []*noise.Peer, reqOpcode byte, req []byte, waitForResponse bool) [][]byte {
	resc := make(chan broadcastResponse, len(peers))
	defer close(resc)

	for i, peer := range peers {
		b.bus <- broadcastPayload{
			order:           i,
			requestOpcode:   reqOpcode,
			body:            req,
			peer:            peer,
			res:             resc,
			waitForResponse: waitForResponse,
		}
	}

	responses := make([][]byte, len(peers))
	for i := 0; i < len(peers); i++ {
		select {
		case <-ctx.Done():
			return nil
		case t := <-resc:
			responses[t.order] = t.body
		}
	}

	return responses
}

func (b *broadcaster) Stop() {
	b.cancel()
	b.wg.Wait()

	close(b.bus)
}
