package wavelet

import "sync"

// Transactions is NOT thread safe.
type Transactions struct {
	buffer map[TransactionID]*Transaction
	mu     sync.RWMutex
}

func NewTransactions() *Transactions {
	return &Transactions{
		buffer: make(map[TransactionID]*Transaction),
	}
}

func (t *Transactions) Add(tx *Transaction) {
	t.mu.Lock()
	t.buffer[tx.ID] = tx
	t.mu.Unlock()
}

// Will return nil if the transaction does not exist.
func (t *Transactions) Get(id TransactionID) *Transaction {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.buffer[id]
}

func (t *Transactions) Has(id TransactionID) bool {
	return t.Get(id) != nil
}

func (t *Transactions) Delete(id TransactionID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.buffer, id)
}

func (t *Transactions) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return len(t.buffer)
}

func (t *Transactions) Iterate(callback func(tx *Transaction)) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, v := range t.buffer {
		callback(v)
	}
}

// For future use, locks the mutex and exposes the raw map (pointer).
func (t *Transactions) Acquire(callback func(buf map[TransactionID]*Transaction)) {
	t.mu.Lock()
	defer t.mu.Unlock()

	callback(t.buffer)
}
