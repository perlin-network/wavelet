package wavelet

// Transactions is NOT thread safe.
type Transactions struct {
	buffer map[TransactionID]*Transaction
}

func NewTransactions() *Transactions {
	return &Transactions{
		buffer: make(map[TransactionID]*Transaction),
	}
}

func (t *Transactions) Add(tx *Transaction) {
	t.buffer[tx.ID] = tx
}

// Will return nil if the transaction does not exist.
func (t *Transactions) Get(id TransactionID) *Transaction {
	return t.buffer[id]
}

func (t *Transactions) Has(id TransactionID) bool {
	_, exist := t.buffer[id]
	return exist
}

func (t *Transactions) Delete(id TransactionID) {
	delete(t.buffer, id)
}

func (t *Transactions) Len() int {
	return len(t.buffer)
}

func (t *Transactions) Iterate(callback func(tx *Transaction)) {
	for _, v := range t.buffer {
		callback(v)
	}
}
