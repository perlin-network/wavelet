package processor

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/params"
)

type CreateContractProcessor struct {
}

func (p *CreateContractProcessor) OnApplyTransaction(ctx *wavelet.TransactionContext) error {
	contract := ctx.LoadAccount(ctx.Transaction.Id)
	contract.Store(params.KeyContractCode, ctx.Transaction.Payload)

	return nil
}
