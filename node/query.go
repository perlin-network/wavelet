package node

import (
	"context"
	"github.com/gogo/protobuf/proto"
	"sync"
	"time"

	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/wavelet/params"
	"github.com/pkg/errors"
)

const (
	queryTimeout = 10 * time.Second
)

var (
	ErrPrecommit = errors.New("failed to precommit tx")
)

type query struct {
	*Wavelet
	sybil
}

func (q query) Query(wired *wire.Transaction) error {
	addresses := q.randomlySelectPeers(params.ConsensusK)

	var wg sync.WaitGroup
	wg.Add(len(addresses))

	responses := make([]bool, len(addresses))

	for i, address := range addresses {
		go func(i int, address string) {
			defer wg.Done()

			client, err := q.net.Client(address)

			if err != nil {
				responses[i] = false
				return
			}

			response, err := func() (proto.Message, error) {
				ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
				defer cancel()
				return client.Request(ctx, wired)
			}()

			if err != nil {
				responses[i] = false
				return
			}

			if response, ok := response.(*QueryResponse); ok && response.StronglyPreferred {
				responses[i] = true
				return
			}

			responses[i] = false
		}(i, address.Address)
	}

	wg.Wait()

	positives := q.weigh(addresses, responses, wired)

	if positives < params.ConsensusAlpha {
		return errors.Wrapf(ErrPrecommit, "expected >= %.2f alpha; got only %.2f positives", params.ConsensusAlpha, positives)
	}

	return nil
}
