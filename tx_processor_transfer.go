package wavelet

import (
	"github.com/perlin-network/wavelet/payload"
	"github.com/pkg/errors"
)

var _ TransactionProcessor = (*TransferProcessor)(nil)

type TransferProcessor struct{}

func (TransferProcessor) OnApplyTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

	reader := payload.NewReader(tx.Payload)

	var recipient [PublicKeySize]byte

	_, err := reader.Read(recipient[:])
	if err != nil {
		return errors.Wrap(err, "transfer: failed to decode recipient address")
	}

	amount, err := reader.ReadUint64()
	if err != nil {
		return errors.Wrap(err, "transfer: failed to decode amount to transfer")
	}

	senderBalance, _ := ctx.ReadAccountBalance(tx.Sender)

	if senderBalance < amount {
		return errors.Errorf("transfer: not enough balance, wanting %d PERLs", amount)
	}

	ctx.WriteAccountBalance(tx.Sender, senderBalance-amount)

	recipientBalance, _ := ctx.ReadAccountBalance(recipient)
	ctx.WriteAccountBalance(recipient, recipientBalance+amount)

	if _, isContract := ctx.ReadAccountContractCode(tx.ID); !isContract {
		return nil
	}

	executor, err := NewContractExecutor(ctx, 50000000)
	if err != nil {
		return errors.Wrap(err, "transfer: failed to load and init smart contract vm")
	}
	executor.EnableLogging = true

	if reader.Len() > 0 {
		funcName, err := reader.ReadString()
		if err != nil {
			return err
		}

		funcParams, err := reader.ReadBytes()
		if err != nil {
			return err
		}

		err = executor.Run(amount, funcName, funcParams...)
	} else {
		err = executor.Run(amount, "on_money_received")
	}

	if err != nil && errors.Cause(err) != ErrContractFunctionNotFound {
		return errors.Wrap(err, "transfer: failed to execute smart contract method")
	}

	return nil
}
