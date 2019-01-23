package processor

import (
	"encoding/binary"
	"encoding/hex"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
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

	if len(ctx.Tx.Payload[40:]) > 0 {
		if code, ok := recipient.Load(params.KeyContractCode); ok {
			nameLen := int(ctx.Tx.Payload[40])
			if len(ctx.Tx.Payload[41:]) < nameLen {
				return errors.New("invalid contract invocation")
			}
			name := string(ctx.Tx.Payload[41 : 41+nameLen])
			payload := ctx.Tx.Payload[41+nameLen:]
			executor := wavelet.NewContractExecutor(recipient, senderID, payload, wavelet.ContractGasPolicy{nil, 100000})
			err := executor.Run(code, name)
			if err != nil {
				return errors.Wrap(err, "smart contract failed")
			}
		} else {
			log.Debug().Msg("junk data after transaction payload")
		}
	}

	return nil
}
