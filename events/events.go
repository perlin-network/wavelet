package events

type transactionUpdate struct {
	ID []byte `json:"id"`
}

type TransactionAcceptedEvent transactionUpdate
type TransactionAppliedEvent transactionUpdate
type TransactionFailedEvent transactionUpdate

func NewTransactionAcceptedEvent(id []byte) *TransactionAcceptedEvent {
	return &TransactionAcceptedEvent{
		ID: id,
	}
}

func NewTransactionAppliedEvent(id []byte) *TransactionAppliedEvent {
	return &TransactionAppliedEvent{
		ID: id,
	}
}

func NewTransactionFailedEvent(id []byte) *TransactionFailedEvent {
	return &TransactionFailedEvent{
		ID: id,
	}
}

type AccountUpdateEvent struct {
	Account string            `json:"account"`
	Nonce   uint64            `json:"nonce"`
	Updates map[string][]byte `json:"updates"`
}
