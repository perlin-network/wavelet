package events

import (
	"reflect"
	"sync"
	"sync/atomic"
)

var globalHub = NewPubsubHub()

func Publish(topic interface{}, data interface{}) {
	globalHub.Publish(topic, data)
}

func Subscribe(topic interface{}, cb interface{}) {
	globalHub.Subscribe(topic, cb)
}

func SubscribeAsync(topic interface{}, cb interface{}) {
	globalHub.SubscribeAsync(topic, cb)
}

type Subscriber func(interface{})

type PubsubHub struct {
	sync.Mutex
	channels map[reflect.Type]*Channel
}

type Channel struct {
	topics map[interface{}]*SubscribeInfo
}

type SubscribeInfo struct {
	subscribers      sync.Map // uint64 -> Subscriber
	prevSubscriberID uint64
}

func NewPubsubHub() *PubsubHub {
	return &PubsubHub{
		channels: make(map[reflect.Type]*Channel),
	}
}

func (h *PubsubHub) getEndpoint(ty reflect.Type, topic interface{}) *SubscribeInfo {
	h.Lock()
	defer h.Unlock()

	ch, ok := h.channels[ty]
	if !ok {
		ch = newChannel()
		h.channels[ty] = ch
	}

	ep, ok := ch.topics[topic]
	if !ok {
		ep = newSubscribeInfo()
		ch.topics[topic] = ep
	}

	return ep
}

func (h *PubsubHub) Publish(topic interface{}, data interface{}) {
	ep := h.getEndpoint(reflect.TypeOf(data), topic)
	ep.publish(data)
}

func (h *PubsubHub) Subscribe(topic interface{}, cb interface{}) {
	h.doSubscribe(topic, cb, true)
}

func (h *PubsubHub) SubscribeAsync(topic interface{}, cb interface{}) {
	h.doSubscribe(topic, cb, false)
}

func (h *PubsubHub) doSubscribe(topic interface{}, cb interface{}, locked bool) {
	cbVal := reflect.ValueOf(cb)

	if cbVal.Kind() != reflect.Func {
		panic("expected func for callback")
	}

	if cbVal.Type().NumIn() != 1 {
		panic("expected exactly one argument for callback")
	}

	if cbVal.Type().NumOut() != 1 || cbVal.Type().Out(0).Kind() != reflect.Bool {
		panic("expected exactly one boolean value for callback return")
	}

	ty := cbVal.Type().In(0)
	ep := h.getEndpoint(ty, topic)
	id := atomic.AddUint64(&ep.prevSubscriberID, 1)
	mutex := &sync.Mutex{}
	runnable := true

	ep.subscribers.Store(id, Subscriber(func(msg interface{}) {
		if locked {
			mutex.Lock()
			defer mutex.Unlock()
			if !runnable {
				return
			}
		}

		ret := cbVal.Call([]reflect.Value{reflect.ValueOf(msg)})
		if ret[0].Bool() == false {
			ep.subscribers.Delete(id)
			runnable = false
		}
	}))
}

func newSubscribeInfo() *SubscribeInfo {
	return &SubscribeInfo{}
}

func newChannel() *Channel {
	return &Channel{
		topics: make(map[interface{}]*SubscribeInfo),
	}
}

func (s *SubscribeInfo) publish(msg interface{}) {
	s.subscribers.Range(func(_ interface{}, _sub interface{}) bool {
		sub := _sub.(Subscriber)
		sub(msg)
		return true
	})
}
