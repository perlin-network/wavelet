package wavelet

import (
	"sync"
)

const (
	shardsCount = 64
)

// TransactionMap is a sharded map with TransactionID as its key
type TransactionMap [shardsCount]*transactionMapShard

type transactionMapShard struct {
	sync.Mutex

	items map[TransactionID]interface{}
}

func NewTransactionMap() TransactionMap {
	var shards TransactionMap
	for i := uint64(0); i < shardsCount; i++ {
		shards[i] = &transactionMapShard{
			items: make(map[TransactionID]interface{}, 2048),
		}
	}

	return shards
}

func (c TransactionMap) Len() uint64 {
	var l uint64

	for i := uint64(0); i < shardsCount; i++ {
		c[i].Lock()
		l += uint64(len(c[i].items))
		c[i].Unlock()
	}

	return l
}

func (c TransactionMap) Iterate(fn func(key TransactionID, value interface{}) bool) {
	for i := uint64(0); i < shardsCount; i++ {
		c[i].Lock()

		for key, value := range c[i].items {
			if !fn(key, value) {
				c[i].Unlock()
				return
			}
		}

		c[i].Unlock()
	}
}

func (c TransactionMap) Get(key TransactionID) (interface{}, bool) {
	shard := c.GetShard(key)

	shard.Lock()
	defer shard.Unlock()

	if tx, exists := shard.items[key]; !exists {
		return nil, false
	} else {
		return tx, true
	}
}

func (c TransactionMap) Put(key TransactionID, value interface{}) {
	shard := c.GetShard(key)

	shard.Lock()
	defer shard.Unlock()

	shard.items[key] = value
}

func (c TransactionMap) Delete(key TransactionID) {
	shard := c.GetShard(key)

	shard.Lock()
	defer shard.Unlock()

	delete(shard.items, key)
}

func (c TransactionMap) GetShard(key TransactionID) (shard *transactionMapShard) {
	// Use the first byte as the shard ID
	shardID := uint64(key[0]) % shardsCount
	return c[shardID]
}
