package worker

import "sync"

type Pool struct {
	bus    chan func()
	stop   chan struct{}
	wg     sync.WaitGroup
	closed bool
}

func NewWorkerPool() *Pool {
	return &Pool{
		stop: make(chan struct{}),
		bus:  make(chan func()),
	}
}

func (p *Pool) Start(workers int) {
	p.wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer p.wg.Done()
			for {
				select {
				case job := <-p.bus:
					if job != nil {
						job()
					}
				case <-p.stop:
					return
				}
			}
		}()
	}
}

func (p *Pool) Queue(job func()) {
	p.bus <- job
}

func (p *Pool) Stop() {
	if p.closed {
		return
	}

	close(p.bus)

	close(p.stop)
	p.wg.Wait()

	p.closed = true
}
