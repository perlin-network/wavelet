package wavelet

import (
	"encoding/hex"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/wavelet/params"
	"github.com/pkg/errors"
)

type TransactionContext struct {
	rewarded            bool
	ledger              *Ledger
	accounts            map[string]*Account
	pendingTransactions []*database.Transaction
	Tx                  *database.Transaction
	firstTx             *database.Transaction
}

type TransactionProcessor interface {
	OnApplyTransaction(ctx *TransactionContext) error
}

func (c *TransactionContext) NewAccount(publicKey string) *Account {
	if acct, ok := c.accounts[publicKey]; ok {
		return acct
	}

	acct := NewAccount(c.ledger, []byte(publicKey))
	c.accounts[publicKey] = acct
	return acct
}

func (c *TransactionContext) SendTransaction(tx *database.Transaction) {
	c.pendingTransactions = append(c.pendingTransactions, tx)
}

func (c *TransactionContext) Reward() error {
	if c.rewarded {
		return nil
	}
	c.rewarded = true

	rewardee, err := c.ledger.randomlySelectValidator(c.firstTx, params.ValidatorRewardAmount, params.ValidatorRewardDepth)
	if err != nil {
		return err
	}

	firstSenderID, err := hex.DecodeString(c.firstTx.Sender)
	if err != nil {
		return err
	}
	sender := c.NewAccount(string(firstSenderID))

	rewardeeID, err := hex.DecodeString(rewardee)
	if err != nil {
		return err
	}
	recipient := c.NewAccount(string(rewardeeID))

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
		ledger:   ledger,
		accounts: make(map[string]*Account),
		Tx:       tx,
		firstTx:  tx,
	}
}

func (c *TransactionContext) run(processor TransactionProcessor) error {
	pending := []*database.Transaction{c.Tx}

	for len(pending) > 0 {
		for _, tx := range pending {
			c.Tx = tx
			err := processor.OnApplyTransaction(c)
			if err != nil {
				return errors.Wrap(err, "failed to apply transaction")
			}
		}
		pending = c.pendingTransactions
		c.pendingTransactions = nil
	}

	for _, acct := range c.accounts {
		acct.writeback()
	}
	return nil
}
