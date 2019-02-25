package wavelet

var _ TransactionProcessor = (*NopProcessor)(nil)

type NopProcessor struct{}

func (NopProcessor) OnApplyTransaction(ctx *TransactionContext) error {
	return nil
}
