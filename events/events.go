package events

type transactionUpdate struct {
	ID string `json:"id"`
}

type TransactionAcceptedEvent transactionUpdate
type TransactionAppliedEvent transactionUpdate
type TransactionFailedEvent transactionUpdate

type AccountUpdateEvent struct {
	Account string            `json:"account"`
	Nonce   uint64            `json:"nonce"`
	Updates map[string][]byte `json:"updates"`
}
