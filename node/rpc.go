package node

import (
	"context"
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"math/rand"
	"sync"
	"time"
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
	responseOpcode  byte
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
							payload.peer.SendWithTimeout(payload.requestOpcode, payload.body, 1*time.Second)
							return
						}

						mux := payload.peer.Mux()
						defer mux.Close()

						if err := mux.SendWithTimeout(payload.requestOpcode, payload.body, 1*time.Second); err != nil {
							return
						}

						select {
						case w := <-mux.Recv(payload.responseOpcode):
							res.body = w.Bytes()
						case <-time.After(sys.QueryTimeout):
							fmt.Println("timed out reading from mux")
						}
					}()
				}
			}
		}(ctx)
	}

	return &b
}

func (b *broadcaster) Broadcast(
	ctx context.Context, peers []*noise.Peer, reqOpcode byte, resOpcode byte, req []byte, waitForResponse bool,
) [][]byte {
	resc := make(chan broadcastResponse, len(peers))
	defer close(resc)

	for i, peer := range peers {
		b.bus <- broadcastPayload{
			order:           i,
			requestOpcode:   reqOpcode,
			responseOpcode:  resOpcode,
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
