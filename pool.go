package wavelet

import (
	"github.com/phf/go-queue/queue"
	"sync"
)

var queuePool sync.Pool

func AcquireQueue() *queue.Queue {
	q := queuePool.Get()

	if q == nil {
		q = queue.New()
	}

	return q.(*queue.Queue)
}

func ReleaseQueue(q *queue.Queue) {
	q.Init()
	queuePool.Put(q)
}
