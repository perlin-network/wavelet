package processor

import (
	"github.com/perlin-network/wavelet"
)

type NopProcessor struct {
}

func (p *NopProcessor) Tag() byte {
	return 0x00
}

func (p *NopProcessor) OnApplyTransaction(ctx *wavelet.TransactionContext) error {
	return nil
}
