package wavelet

import (
	"fmt"
	"github.com/perlin-network/wavelet/store"
	"sync"
)

type RoundManager struct {
	limit uint8
	count uint64
	storage store.KV
	latest uint32
	oldest uint32
	buff []*Round
	sync.RWMutex
}

func NewRoundManager(limit uint8, storage store.KV) (*RoundManager, error) {
	rm := &RoundManager{
		limit: limit,
		buff: make([]*Round, 0, limit),
		storage: storage,
	}

	rounds, count, latest, oldest, err := LoadRounds(storage)
	if err != nil {
		return rm, err
	}
	fmt.Println(len(rounds), count, latest, oldest)
	rm.buff = rounds
	rm.count = count
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
	rm.RLock()
	count := rm.count
	rm.RUnlock()

	return count
}

func (rm *RoundManager) Save(round *Round, count uint64) (*Round, error) {
	rm.Lock()
	defer rm.Unlock()

	rm.count = count

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

	err := StoreRound(rm.storage, count, *round, rm.latest, rm.oldest, uint8(len(rm.buff)))
	return oldRound, err
}