package wavelet

import "sync"

type WorkerPool struct {
	bus    chan func()
	stop   chan struct{}
	wg     sync.WaitGroup
	closed bool
}

func NewWorkerPool() *WorkerPool {
	return &WorkerPool{
		stop: make(chan struct{}),
		bus:  make(chan func()),
	}
}

func (wp *WorkerPool) Start(workers int) {
	wp.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wp.wg.Done()
			for {
				select {
				case job, _ := <-wp.bus:
					if job != nil {
						job()
					}
				case <-wp.stop:
					return
				}
			}
		}()
	}
}

func (wp *WorkerPool) Queue(job func()) {
	wp.bus <- job
}

func (wp *WorkerPool) Stop() {
	if wp.closed {
		return
	}

	close(wp.bus)

	close(wp.stop)
	wp.wg.Wait()

	wp.closed = true
}
