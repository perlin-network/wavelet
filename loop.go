package wavelet

import (
	"github.com/pkg/errors"
	"sync"
)

// eventLoop provides an event loop to reduce mutex contention for a *Ledger instance.
type eventLoop struct {
	sync.Mutex
	ledger    *Ledger
	taskQueue chan *task
}

// LoopHandle provides a handle to an existing, running event loop.
type LoopHandle struct {
	taskQueue chan *task
}

// task is a single job to be executed on an event loop.
type task struct {
	handle     *LoopHandle
	chain      []func(task *task, ledger *Ledger) bool
	lastResult interface{}
	completion chan struct{}
}

func NewEventLoop(ledger *Ledger) *eventLoop {
	return &eventLoop{
		ledger:    ledger,
		taskQueue: make(chan *task, 4096),
	}
}

func (l *eventLoop) RunForever() {
	for t := range l.taskQueue {
		l.Lock()
		t.step(l.ledger)
		l.ledger.RunPeriodicTasks()
		l.Unlock()
	}
}

func (l *eventLoop) Handle() *LoopHandle {
	return &LoopHandle{
		taskQueue: l.taskQueue,
	}
}

func (l *LoopHandle) submit(task *task) error {
	select {
	case l.taskQueue <- task:
		return nil
	default:
		return errors.Errorf("task queue full")
	}
}

func (l *LoopHandle) Do(f func(ledger *Ledger)) {
	l.Task().Push(func(_ *task, ledger *Ledger) bool {
		f(ledger)
		return true
	}).Resume(nil).Wait()
}

func (l *LoopHandle) Task() *task {
	return &task{
		handle:     l,
		completion: make(chan struct{}),
	}
}

func (t *task) Push(f func(task *task, ledger *Ledger) bool) *task {
	t.chain = append(t.chain, f)
	return t
}

func (t *task) step(ledger *Ledger) {
	if len(t.chain) == 0 {
		return
	}

	cont := t.chain[0](t, ledger)

	if cont {
		t.chain = t.chain[1:]
	} else {
		t.chain = nil
	}

	if len(t.chain) == 0 {
		close(t.completion)
	}
}

func (t *task) Resume(result interface{}) *task {
	t.lastResult = result
	t.handle.submit(t)
	return t
}

func (t *task) Wait() {
	<-t.completion
}
