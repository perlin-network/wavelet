package wavelet

import (
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
)

type TransactionProcessor interface {
	OnApplyTransaction(ctx *TransactionContext) error
}

type TransactionContext struct {
	accounts accounts

	balances map[[PublicKeySize]byte]uint64
	stakes   map[[PublicKeySize]byte]uint64

	transactions queue.Queue
	tx           *Transaction
}

func newTransactionContext(accounts accounts, tx *Transaction) *TransactionContext {
	ctx := &TransactionContext{
		accounts: accounts,
		balances: make(map[[PublicKeySize]byte]uint64),
		stakes:   make(map[[PublicKeySize]byte]uint64),

		tx: tx,
	}

	ctx.transactions.PushBack(tx)

	return ctx
}

func (c *TransactionContext) Transaction() Transaction {
	return *c.tx
}

func (c *TransactionContext) SendTransaction(tx *Transaction) {
	c.transactions.PushBack(tx)
}

func (c *TransactionContext) ReadAccountBalance(id [PublicKeySize]byte) (uint64, bool) {
	if balance, ok := c.balances[id]; ok {
		return balance, true
	}

	balance, exists := c.accounts.ReadAccountBalance(id)
	c.balances[id] = balance
	return balance, exists
}

func (c *TransactionContext) ReadAccountStake(id [PublicKeySize]byte) (uint64, bool) {
	if stake, ok := c.stakes[id]; ok {
		return stake, true
	}

	stake, exists := c.accounts.ReadAccountStake(id)
	c.stakes[id] = stake
	return stake, exists
}

func (c *TransactionContext) WriteAccountBalance(id [PublicKeySize]byte, balance uint64) {
	c.balances[id] = balance
}

func (c *TransactionContext) WriteAccountStake(id [PublicKeySize]byte, stake uint64) {
	c.stakes[id] = stake
}

func (c *TransactionContext) apply(processor TransactionProcessor) error {
	for c.transactions.Len() > 0 {
		c.tx = c.transactions.PopFront().(*Transaction)

		err := processor.OnApplyTransaction(c)
		if err != nil {
			return errors.Wrap(err, "failed to apply transaction")
		}
	}

	// If the transaction processor executed properly, apply changes from
	// the transactions context over to our accounts snapshot.

	for id, balance := range c.balances {
		c.accounts.WriteAccountBalance(id, balance)
	}

	for id, stake := range c.stakes {
		c.accounts.WriteAccountStake(id, stake)
	}

	return nil
}
