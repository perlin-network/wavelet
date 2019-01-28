package wavelet

import (
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/wavelet/params"
	"github.com/pkg/errors"
)

type TransactionContext struct {
	ledger              *Ledger
	accounts            map[string]*Account
	pendingTransactions []*database.Transaction
	firstTx             *database.Transaction

	Transaction *database.Transaction
}

type TransactionProcessor interface {
	OnApplyTransaction(ctx *TransactionContext) error
}

func (c *TransactionContext) LoadAccount(publicKey []byte) *Account {
	if account, ok := c.accounts[writeString(publicKey)]; ok {
		return account
	}

	account := LoadAccount(c.ledger.Accounts(), publicKey)
	c.accounts[writeString(publicKey)] = account

	return account
}

func (c *TransactionContext) SendTransaction(tx *database.Transaction) {
	c.pendingTransactions = append(c.pendingTransactions, tx)
}

func (c *TransactionContext) reward() error {
	rewardee, err := c.ledger.randomlySelectValidator(c.firstTx, params.ValidatorRewardAmount, params.ValidatorRewardDepth)
	if err != nil {
		return err
	}

	sender := c.LoadAccount(c.firstTx.Sender)

	recipient := c.LoadAccount(rewardee)

	deducted := params.ValidatorRewardAmount
	if sender.GetBalance() < deducted {
		return errors.Errorf("balance is not enough to pay reward, wanting %d PERLs", deducted)
	}

	sender.SetBalance(sender.GetBalance() - deducted)
	recipient.SetBalance(recipient.GetBalance() + deducted)

	return nil
}

func newTransactionContext(ledger *Ledger, tx *database.Transaction) *TransactionContext {
	return &TransactionContext{
		ledger:      ledger,
		accounts:    make(map[string]*Account),
		Transaction: tx,
		firstTx:     tx,
	}
}

func (c *TransactionContext) run(processor TransactionProcessor) error {
	pending := []*database.Transaction{c.Transaction}

	for len(pending) > 0 {
		for _, tx := range pending {
			c.Transaction = tx
			err := processor.OnApplyTransaction(c)
			if err != nil {
				return errors.Wrap(err, "failed to apply transaction")
			}
		}
		pending = c.pendingTransactions
		c.pendingTransactions = nil
	}

	if err := c.reward(); err != nil {
		return err
	}

	for _, account := range c.accounts {
		c.ledger.Accounts().save(account)
	}
	return nil
}
