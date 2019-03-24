package wavelet

import (
	"github.com/phf/go-queue/queue"
	"sync"
)

var (
	queuePool = sync.Pool{
		New: func() interface{} {
			return queue.New()
		},
	}
)
