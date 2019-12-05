package main

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"

	"github.com/perlin-network/wavelet"
	"golang.org/x/crypto/blake2b"
)

type mapMutex struct {
	sync.Mutex

	items map[wavelet.TransactionID]uint64
}

func (m *mapMutex) Get(key wavelet.TransactionID) uint64 {
	m.Lock()
	defer m.Unlock()

	return m.items[key]
}

func generateKV(n int) ([]wavelet.TransactionID, map[wavelet.TransactionID]uint64) {
	keys := make([]wavelet.TransactionID, n)
	kv := make(map[wavelet.TransactionID]uint64, n)
	buf := make([]byte, 8)

	for i := 0; i < n; i++ {
		rand.Read(buf)
		key := blake2b.Sum256(buf)
		keys[i] = key
		kv[key] = binary.BigEndian.Uint64(buf)
	}

	return keys, kv
}

func setupSyncMap(kv map[wavelet.TransactionID]uint64) sync.Map {
	var m sync.Map
	for k, v := range kv {
		m.Store(k, v)
	}
	return m
}

func setupTransactionMap(kv map[wavelet.TransactionID]uint64) wavelet.TransactionMap {
	m := wavelet.NewTransactionMap()
	for k, v := range kv {
		m.Put(k, v)
	}
	return m
}

func setupMapMutex(kv map[wavelet.TransactionID]uint64) *mapMutex {
	m := &mapMutex{items: make(map[wavelet.TransactionID]uint64, 2048)}
	for k, v := range kv {
		m.items[k] = v
	}
	return m
}

func benchmarkSyncMap(m sync.Map, keys []wavelet.TransactionID, g int) func(b *testing.B) {
	return func(b *testing.B) {
		var wg sync.WaitGroup
		for i := 0; i < g; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for n := 0; n < b.N; n++ {
					_, _ = m.Load(keys[rand.Intn(len(keys))])
				}
			}()
		}

		wg.Wait()
	}
}

func benchmarkTransactionMap(m wavelet.TransactionMap, keys []wavelet.TransactionID, g int) func(b *testing.B) {
	return func(b *testing.B) {
		var wg sync.WaitGroup
		for i := 0; i < g; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for n := 0; n < b.N; n++ {
					_, _ = m.Get(keys[rand.Intn(len(keys))])
				}
			}()
		}

		wg.Wait()
	}
}

func benchmarkMapMutex(m *mapMutex, keys []wavelet.TransactionID, g int) func(b *testing.B) {
	return func(b *testing.B) {
		var wg sync.WaitGroup
		for i := 0; i < g; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for n := 0; n < b.N; n++ {
					_ = m.Get(keys[rand.Intn(len(keys))])
				}
			}()
		}

		wg.Wait()
	}
}

func main() {
	nVals := []int{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		20, 30, 40, 50, 60, 70, 80, 90, 100,
		200, 300, 400, 500, 600, 700, 800, 900, 1000,
	}

	for _, nv := range nVals {
		n := nv * 1000

		keys, kv := generateKV(n)
		mapMutex := setupMapMutex(kv)
		txMap := setupTransactionMap(kv)
		syncMap := setupSyncMap(kv)

		mapMutexResult := testing.Benchmark(benchmarkMapMutex(mapMutex, keys, runtime.NumCPU()))
		txMapResult := testing.Benchmark(benchmarkTransactionMap(txMap, keys, runtime.NumCPU()))
		syncMapResult := testing.Benchmark(benchmarkSyncMap(syncMap, keys, runtime.NumCPU()))

		fmt.Printf("%d,%d,%d,%d\n",
			n, mapMutexResult.NsPerOp(), txMapResult.NsPerOp(), syncMapResult.NsPerOp())
	}
}
