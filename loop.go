package wavelet

import (
	"github.com/pkg/errors"
	"sync"
)

type LedgerLoop struct {
	ledgerLock sync.Mutex
	ledger     *Ledger
	taskQueue  chan *LedgerTask
}

type LedgerLoopHandle struct {
	taskQueue chan *LedgerTask
}

type LedgerTask struct {
	handle     *LedgerLoopHandle
	chain      []func(task *LedgerTask, ledger *Ledger) bool
	lastResult interface{}
	completion chan struct{}
}

func NewLoop(ledger *Ledger) *LedgerLoop {
	return &LedgerLoop{
		ledger:    ledger,
		taskQueue: make(chan *LedgerTask, 4096),
	}
}

func (l *LedgerLoop) RunForever() {
	for t := range l.taskQueue {
		l.ledgerLock.Lock()
		t.step(l.ledger)
		l.ledgerLock.Unlock()
	}
}

func (l *LedgerLoop) Handle() *LedgerLoopHandle {
	return &LedgerLoopHandle{
		taskQueue: l.taskQueue,
	}
}

func (l *LedgerLoopHandle) submit(task *LedgerTask) error {
	select {
	case l.taskQueue <- task:
		return nil
	default:
		return errors.Errorf("task queue full")
	}
}

func (l *LedgerLoopHandle) Atomically(f func(ledger *Ledger)) {
	l.Task().Push(func(_ *LedgerTask, ledger *Ledger) bool {
		f(ledger)
		return true
	}).Resume(nil).Wait()
}

func (l *LedgerLoopHandle) Task() *LedgerTask {
	return &LedgerTask{
		handle:     l,
		completion: make(chan struct{}),
	}
}

func (t *LedgerTask) Push(f func(task *LedgerTask, ledger *Ledger) bool) *LedgerTask {
	t.chain = append(t.chain, f)
	return t
}

func (t *LedgerTask) step(ledger *Ledger) {
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

func (t *LedgerTask) Resume(result interface{}) *LedgerTask {
	t.lastResult = result
	t.handle.submit(t)
	return t
}

func (t *LedgerTask) Wait() {
	<-t.completion
}
