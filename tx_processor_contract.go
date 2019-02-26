package wavelet

var _ TransactionProcessor = (*ContractProcessor)(nil)

type ContractProcessor struct{}

func (ContractProcessor) OnApplyTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

	ctx.WriteAccountContractCode(tx.ID, tx.Payload)
	return nil
}
