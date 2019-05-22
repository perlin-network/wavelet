package wavelet

import (
	"fmt"
	"github.com/perlin-network/wavelet/store"
	"sync"
)

type RoundManager struct {
	limit   uint8
	storage store.KV
	latest  uint32
	oldest  uint32
	buff    []*Round
	sync.RWMutex
}

func NewRoundManager(limit uint8, storage store.KV) (*RoundManager, error) {
	rm := &RoundManager{
		limit:   limit,
		buff:    make([]*Round, 0, limit),
		storage: storage,
	}

	rounds, latest, oldest, err := LoadRounds(storage)
	if err != nil {
		return rm, err
	}

	rm.buff = rounds
	rm.latest = latest
	rm.oldest = oldest

	return rm, nil
}

func (rm *RoundManager) Oldest() *Round {
	rm.RLock()
	round := rm.buff[rm.oldest]
	rm.RUnlock()

	return round
}

func (rm *RoundManager) Latest() *Round {
	rm.RLock()
	round := rm.buff[rm.latest]
	rm.RUnlock()

	return round
}

func (rm *RoundManager) Count() uint64 {
	return rm.Latest().Index
}

func (rm *RoundManager) Save(round *Round) (*Round, error) {
	rm.Lock()
	defer rm.Unlock()

	if len(rm.buff) > 0 {
		rm.latest = (rm.latest + 1) % uint32(rm.limit)

		if rm.oldest == rm.latest {
			rm.oldest = (rm.oldest + 1) % uint32(rm.limit)
		}
	}

	var oldRound *Round
	if uint8(len(rm.buff)) < rm.limit {
		rm.buff = append(rm.buff, round)
	} else {
		oldRound = rm.buff[rm.latest]
		rm.buff[rm.latest] = round
	}

	err := StoreRound(rm.storage, *round, rm.latest, rm.oldest, uint8(len(rm.buff)))
	return oldRound, err
}

func (rm *RoundManager) GetRound(ix uint64) (*Round, error) {
	var round *Round
	rm.RLock()
	for _, r := range rm.buff {
		if ix == r.Index {
			round = r
		}
	}
	rm.RUnlock()

	if round == nil {
		return nil, fmt.Errorf("no round found for index - %d", ix)
	}

	return round, nil
}
