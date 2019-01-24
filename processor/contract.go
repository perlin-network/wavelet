package processor

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/params"
)

type CreateContractProcessor struct {
}

func (p *CreateContractProcessor) OnApplyTransaction(ctx *wavelet.TransactionContext) error {
	contractID := wavelet.ContractID(ctx.Tx.Id)
	contract := ctx.LoadAccount(contractID)
	contract.Store(params.KeyContractCode, ctx.Tx.Payload)

	return nil
}
