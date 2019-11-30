// +build !integration,unit

package wavelet

import (
	"testing"
	"time"
)

func TestWorkerPool(t *testing.T) {
	wp := NewWorkerPool()

	wp.Start(2)
	defer wp.Stop()

	res := make(chan struct{}, 3)

	job := func() {
		select {
		case res <- struct{}{}:
		default:
			t.Fatal()
		}
	}

	for i := 0; i < cap(res); i++ {
		wp.Queue(job)
	}

	for i := 0; i < cap(res); i++ {
		select {
		case <-res:
		case <-time.After(10 * time.Millisecond):
			t.Fatal()
		}
	}
}
