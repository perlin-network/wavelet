package processor

import "github.com/perlin-network/wavelet"

var _ wavelet.TransactionProcessor = (*NopProcessor)(nil)

type NopProcessor struct{}

func (NopProcessor) OnApplyTransaction(ctx *wavelet.TransactionContext) error {
	return nil
}
