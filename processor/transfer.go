package processor

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/params"
	"github.com/pkg/errors"
)

type TransferProcessor struct {
}

func (p *TransferProcessor) OnApplyTransaction(ctx *wavelet.TransactionContext) error {
	sender := ctx.LoadAccount(ctx.Transaction.Sender)

	reader := wavelet.NewPayloadReader(ctx.Transaction.Payload)

	recipientAddress, err := reader.ReadBytes()
	if err != nil {
		return errors.Wrap(err, "failed to decode recipient address")
	}
	recipient := ctx.LoadAccount([]byte(recipientAddress))

	amount, err := reader.ReadUint64()
	if err != nil {
		return errors.Wrap(err, "failed to decode amount to transfer")
	}

	if sender.GetBalance() < amount {
		return errors.Errorf("not enough balance, wanting %d PERLs", amount)
	}

	sender.SetBalance(sender.GetBalance() - amount)
	recipient.SetBalance(recipient.GetBalance() + amount)

	if _, ok := recipient.Load(params.KeyContractCode); ok {
		writer := wavelet.NewPayloadBuilder()
		writer.WriteBytes([]byte(ctx.Transaction.Id))
		writer.WriteBytes(ctx.Transaction.Sender)
		writer.WriteUint64(amount)

		executor := wavelet.NewContractExecutor(recipient, ctx.Transaction.Sender, writer.Bytes(), wavelet.ContractGasPolicy{GasLimit: 50000000})
		executor.EnableLogging = true

		if reader.Len() > 0 {
			funcName, err := reader.ReadUTF8String()
			if err != nil {
				return err
			}

			funcParams, err := reader.ReadBytes()
			if err != nil {
				return err
			}

			err = executor.Run(funcName, funcParams...)
		} else {
			err = executor.Run("on_money_received")
		}

		if err != nil {
			return errors.Wrap(err, "failed to execute smart contract method")
		}
	}

	return nil
}
