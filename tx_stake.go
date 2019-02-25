package wavelet

import (
	"github.com/perlin-network/wavelet/payload"
	"github.com/pkg/errors"
)

var _ TransactionProcessor = (*StakeProcessor)(nil)

type StakeProcessor struct{}

func (StakeProcessor) OnApplyTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

	raw, err := payload.NewReader(tx.Payload).ReadUint64()

	if err != nil {
		return errors.Wrap(err, "stake: failed to decode stake delta amount")
	}

	balance, _ := ctx.ReadAccountBalance(tx.Sender)
	stake, _ := ctx.ReadAccountStake(tx.Sender)

	delta := int64(raw)

	if delta >= 0 {
		delta := uint64(delta)

		if balance < delta {
			return errors.New("stake: balance < delta")
		}

		ctx.WriteAccountBalance(tx.Sender, balance-delta)
		ctx.WriteAccountStake(tx.Sender, stake+delta)
	} else {
		delta := uint64(-delta)

		if stake < delta {
			return errors.New("stake: stake < delta")
		}

		ctx.WriteAccountBalance(tx.Sender, stake-delta)
		ctx.WriteAccountBalance(tx.Sender, balance+delta)
	}

	return nil
}
