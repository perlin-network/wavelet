package processor

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/payload"
	"github.com/pkg/errors"
)

type StakeProcessor struct {
}

func (p *StakeProcessor) OnApplyTransaction(ctx *wavelet.TransactionContext) error {
	raw, err := payload.NewReader(ctx.Transaction.Payload).ReadUint64()

	if err != nil {
		return errors.Wrap(err, "failed to decode stake delta amount")
	}

	delta := int64(raw)

	acct := ctx.LoadAccount(ctx.Transaction.Sender)

	if delta >= 0 {
		delta := uint64(delta)
		balance := acct.GetBalance()
		if balance < delta {
			return errors.New("balance < delta")
		}
		acct.SetBalance(balance - delta)
		acct.SetStake(acct.GetStake() + delta)
	} else {
		delta := uint64(-delta)
		stake := acct.GetStake()
		if stake < delta {
			return errors.New("stake < delta")
		}
		acct.SetStake(stake - delta)
		acct.SetBalance(acct.GetBalance() + delta)
	}

	return nil
}
