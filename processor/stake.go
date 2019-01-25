package processor

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
)

type StakeProcessor struct {
}

func (p *StakeProcessor) OnApplyTransaction(ctx *wavelet.TransactionContext) error {
	if len(ctx.Transaction.Payload) != 8 {
		return errors.New("expecting an int64")
	}

	delta := int64(binary.LittleEndian.Uint64(ctx.Transaction.Payload))

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
