package processor

import (
	"encoding/binary"
	"encoding/hex"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/params"
	"github.com/pkg/errors"
)

type GenericProcessor struct {
}

func (p *GenericProcessor) OnApplyTransaction(ctx *wavelet.TransactionContext) error {
	senderID, err := hex.DecodeString(ctx.Tx.Sender)
	if err != nil {
		return err
	}
	sender := ctx.NewAccount(string(senderID))

	if len(ctx.Tx.Payload) < 40 /* 32 (recipient) + 8 (amount) */ {
		return errors.New("payload is too short")
	}

	recipient := ctx.NewAccount(string(ctx.Tx.Payload[:32]))
	amount := binary.LittleEndian.Uint64(ctx.Tx.Payload[32:40])

	if sender.GetBalance() < amount {
		return errors.Errorf("no enough balance, wanting %d PERLs", amount)
	}

	sender.SetBalance(sender.GetBalance() - amount)
	recipient.SetBalance(recipient.GetBalance() + amount)

	if _, ok := recipient.Load(params.KeyContractCode); ok {
		payloadHeaderBuilder := wavelet.NewPayloadBuilder()
		payloadHeaderBuilder.WriteBytes([]byte(ctx.Tx.Id))
		payloadHeaderBuilder.WriteBytes(senderID)
		payloadHeaderBuilder.WriteUint64(amount)

		if len(ctx.Tx.Payload[40:]) > 0 {
			reader := wavelet.NewPayloadReader(ctx.Tx.Payload[40:])
			name, err := reader.ReadUTF8String()
			if err != nil {
				return err
			}
			invocationPayload, err := reader.ReadBytes()
			if err != nil {
				return err
			}
			executor := wavelet.NewContractExecutor(recipient, senderID, append(payloadHeaderBuilder.Build(), invocationPayload...), wavelet.ContractGasPolicy{nil, 50000000})
			executor.EnableLogging = true
			err = executor.Run(name)
			if err != nil {
				return errors.Wrap(err, "smart contract failed")
			}
		} else {
			executor := wavelet.NewContractExecutor(recipient, senderID, payloadHeaderBuilder.Build(), wavelet.ContractGasPolicy{nil, 50000000})
			executor.EnableLogging = true
			err := executor.Run("on_money_received")
			if err != nil {
				return errors.Wrap(err, "smart contract failed")
			}
		}
	}

	return nil
}
