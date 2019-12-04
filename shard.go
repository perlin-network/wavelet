package wavelet

import (
	"sync"
)

// TransactionMap is a sharded map with TransactionID as its key
type TransactionMap map[uint64]*transactionMapShard

type transactionMapShard struct {
	items map[TransactionID]interface{}
	lock  *sync.RWMutex
}

const (
	shardsCount = 64
)

func NewTransactionMap() TransactionMap {
	shards := make(TransactionMap, shardsCount)

	for i := uint64(0); i < shardsCount; i++ {
		shards[i] = &transactionMapShard{
			items: make(map[TransactionID]interface{}, 2048),
			lock:  new(sync.RWMutex),
		}
	}

	return shards
}

func (c TransactionMap) Len() uint64 {
	// Lock all shards
	for i := uint64(0); i < shardsCount; i++ {
		c[i].lock.RLock()
		defer c[i].lock.RUnlock()
	}

	var l uint64
	for i := uint64(0); i < shardsCount; i++ {
		l += uint64(len(c[i].items))
	}

	return l
}

func (c TransactionMap) Iterate(fn func(key TransactionID, value interface{}) bool) {
	// Lock all shards
	for i := uint64(0); i < shardsCount; i++ {
		c[i].lock.RLock()
		defer c[i].lock.RUnlock()
	}

	for i := uint64(0); i < shardsCount; i++ {
		for key, value := range c[i].items {
			if !fn(key, value) {
				return
			}
		}
	}
}

func (c TransactionMap) Get(key TransactionID) (interface{}, bool) {
	shard := c.GetShard(key)
	shard.lock.RLock()
	defer shard.lock.RUnlock()

	if tx, exists := shard.items[key]; !exists {
		return nil, false
	} else {
		return tx, true
	}
}

func (c TransactionMap) Put(key TransactionID, value interface{}) {
	shard := c.GetShard(key)

	shard.lock.Lock()
	defer shard.lock.Unlock()

	shard.items[key] = value
}

func (c TransactionMap) Delete(key TransactionID) {
	shard := c.GetShard(key)

	shard.lock.Lock()
	defer shard.lock.Unlock()

	delete(shard.items, key)
}

func (c TransactionMap) GetShard(key TransactionID) (shard *transactionMapShard) {
	// Use the first byte as the shard ID
	shardID := uint64(key[0]) % shardsCount
	return c[shardID]
}
