package wavelet

import (
	"fmt"
	"sync"

	"github.com/perlin-network/wavelet/store"
)

type Blocks struct {
	sync.RWMutex

	store  store.KV
	buffer []*Block

	latest uint32
	oldest uint32
	limit  uint8
}

func NewBlocks(store store.KV, limit uint8) (*Blocks, error) {
	r := &Blocks{
		store:  store,
		buffer: make([]*Block, 0, limit),
		limit:  limit,
	}

	blocks, latest, oldest, err := LoadBlocks(store)
	if err != nil {
		return r, err
	}

	r.buffer = blocks
	r.latest = latest
	r.oldest = oldest

	return r, nil
}

func (b *Blocks) Oldest() *Block {
	b.RLock()
	block := b.buffer[b.oldest]
	b.RUnlock()

	return block
}

func (b *Blocks) Latest() *Block {
	b.RLock()
	block := b.buffer[b.latest]
	b.RUnlock()

	return block
}

func (b *Blocks) Count() uint64 {
	return b.Latest().Index
}

// Save stores the block on disk. It returns the oldest
// block that should be pruned, if any.
func (b *Blocks) Save(block *Block) (*Block, error) {
	b.Lock()

	if len(b.buffer) > 0 {
		b.latest = (b.latest + 1) % uint32(b.limit)

		if b.oldest == b.latest {
			b.oldest = (b.oldest + 1) % uint32(b.limit)
		}
	}

	var oldBlock *Block

	if uint8(len(b.buffer)) < b.limit {
		b.buffer = append(b.buffer, block)
	} else {
		oldBlock = b.buffer[b.latest]
		b.buffer[b.latest] = block
	}

	err := StoreBlock(b.store, *block, b.latest, b.oldest, uint8(len(b.buffer)))

	b.Unlock()

	return oldBlock, err
}

func (b *Blocks) GetByIndex(ix uint64) (*Block, error) {
	var block *Block

	b.RLock()
	for _, r := range b.buffer {
		if ix == r.Index {
			block = r
			break
		}
	}
	b.RUnlock()

	if block == nil {
		return nil, fmt.Errorf("no block found for index - %d", ix)
	}

	return block, nil
}

func (b *Blocks) GetAll() []*Block {
	b.RLock()

	blocks := make([]*Block, 0, len(b.buffer))
	blocks = append(blocks, b.buffer...)

	b.RUnlock()

	return blocks
}
