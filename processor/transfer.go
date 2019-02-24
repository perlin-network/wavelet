package processor

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/payload"
	"github.com/pkg/errors"
)

var _ wavelet.TransactionProcessor = (*TransferProcessor)(nil)

type TransferProcessor struct{}

func (TransferProcessor) OnApplyTransaction(ctx *wavelet.TransactionContext) error {
	tx := ctx.Transaction()

	reader := payload.NewReader(tx.Payload)

	recipientBuf, err := reader.ReadBytes()
	if err != nil {
		return errors.Wrap(err, "transfer: failed to decode recipient address")
	}

	var recipient [wavelet.PublicKeySize]byte
	copy(recipient[:], recipientBuf)

	amount, err := reader.ReadUint64()
	if err != nil {
		return errors.Wrap(err, "transfer: failed to decode amount to transfer")
	}

	senderBalance, _ := ctx.ReadAccountBalance(tx.Sender)
	recipientBalance, _ := ctx.ReadAccountBalance(recipient)

	if senderBalance < amount {
		return errors.Errorf("transfer: not enough balance, wanting %d PERLs", amount)
	}

	ctx.WriteAccountBalance(tx.Sender, senderBalance-amount)
	ctx.WriteAccountBalance(recipient, recipientBalance+amount)

	// TODO(kenta): smart contract execution on transfer

	return nil
}
