package events

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

type MessageA struct{}
type MessageB struct{}

func doConcurrentPubsub(async bool) uint64 {
	const IterationCount = 10000

	hub := NewPubsubHub()
	wg := &sync.WaitGroup{}
	var concCount uint64
	var workerCount int64

	wg.Add(IterationCount)

	var subscribeFn func(interface{}, interface{})
	if async {
		subscribeFn = hub.SubscribeAsync
	} else {
		subscribeFn = hub.Subscribe
	}

	subscribeFn((*MessageA)(nil), func(msg *MessageA) bool {
		newWorkerCount := atomic.AddInt64(&workerCount, 1)
		if newWorkerCount > 1 {
			atomic.AddUint64(&concCount, 1)
		}
		for i := 0; i < 100; i++ {
		}
		atomic.AddInt64(&workerCount, -1)
		wg.Done()
		return true
	})
	for i := 0; i < IterationCount; i++ {
		go hub.Publish((*MessageA)(nil), &MessageA{})
	}
	wg.Wait()
	return concCount
}

func TestSyncSubscribe(t *testing.T) {
	if doConcurrentPubsub(false) != 0 {
		panic("concurrency != 0 for sync")
	}
}

func TestAsyncSubscribe(t *testing.T) {
	if doConcurrentPubsub(true) == 0 {
		panic("concurrency == 0 for async")
	}
}

func TestSubscribeTypeMismatch(t *testing.T) {
	expectPanic := func() {
		if recover() == nil {
			panic("expected panic")
		}
	}
	hub := NewPubsubHub()
	// func() {
	// 	defer expectPanic()
	// 	hub.Subscribe((*MessageA)(nil), func(msg *MessageB) bool { return true })
	// }()
	func() {
		defer expectPanic()
		hub.Subscribe((*MessageA)(nil), func(msg *MessageA) {})
	}()
	func() {
		defer expectPanic()
		hub.Subscribe((*MessageA)(nil), func(msg *MessageA) int { return 0 })
	}()
}

func TestBasicPubsub(t *testing.T) {
	hub := NewPubsubHub()

	aCount := 0
	bCount := 0

	hub.Subscribe((*MessageA)(nil), func(msg *MessageA) bool {
		if msg == nil {
			return false
		}
		aCount++
		return true
	})

	hub.Subscribe((*MessageB)(nil), func(msg *MessageB) bool {
		if msg == nil {
			return false
		}
		bCount++
		return true
	})

	for i := 0; i < 60000; i++ {
		hub.Publish((*MessageA)(nil), &MessageA{})
	}
	hub.Publish(nil, (*MessageA)(nil))

	for i := 0; i < 50000; i++ {
		hub.Publish((*MessageB)(nil), &MessageB{})
	}
	hub.Publish(nil, (*MessageB)(nil))

	if aCount != 60000 || bCount != 50000 {
		panic(fmt.Errorf("unexpected aCount/bCount; aCount = %d, bCount = %d", aCount, bCount))
	}
}
