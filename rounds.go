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
	"fmt"
	"github.com/perlin-network/wavelet/store"
	"sync"
)

type Rounds struct {
	sync.RWMutex

	store  store.KV
	buffer []*Round

	latest uint32
	oldest uint32
	limit  uint8
}

func NewRounds(store store.KV, limit uint8) (*Rounds, error) {
	r := &Rounds{
		store:  store,
		buffer: make([]*Round, 0, limit),
		limit:  limit,
	}

	rounds, latest, oldest, err := LoadRounds(store)
	if err != nil {
		return r, err
	}

	r.buffer = rounds
	r.latest = latest
	r.oldest = oldest

	return r, nil
}

func (r *Rounds) Oldest() *Round {
	r.RLock()
	round := r.buffer[r.oldest]
	r.RUnlock()

	return round
}

func (r *Rounds) Latest() *Round {
	r.RLock()
	round := r.buffer[r.latest]
	r.RUnlock()

	return round
}

func (r *Rounds) Count() uint64 {
	return r.Latest().Index
}

func (r *Rounds) Save(round *Round) (*Round, error) {
	r.Lock()

	if len(r.buffer) > 0 {
		r.latest = (r.latest + 1) % uint32(r.limit)

		if r.oldest == r.latest {
			r.oldest = (r.oldest + 1) % uint32(r.limit)
		}
	}

	var oldRound *Round

	if uint8(len(r.buffer)) < r.limit {
		r.buffer = append(r.buffer, round)
	} else {
		oldRound = r.buffer[r.latest]
		r.buffer[r.latest] = round
	}

	err := StoreRound(r.store, *round, r.latest, r.oldest, uint8(len(r.buffer)))

	r.Unlock()

	return oldRound, err
}

func (r *Rounds) GetByIndex(ix uint64) (*Round, error) {
	var round *Round

	r.RLock()
	for _, r := range r.buffer {
		if ix == r.Index {
			round = r
			break
		}
	}
	r.RUnlock()

	if round == nil {
		return nil, fmt.Errorf("no round found for index - %d", ix)
	}

	return round, nil
}
