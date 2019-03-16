package wavelet

import (
	"github.com/perlin-network/wavelet/common"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
)

type TransactionProcessor func(ctx *TransactionContext) error

type TransactionContext struct {
	accounts accounts

	balances map[common.AccountID]uint64
	stakes   map[common.AccountID]uint64

	contracts        map[common.TransactionID][]byte
	contractNumPages map[common.TransactionID]uint64
	contractPages    map[common.TransactionID]map[uint64][]byte

	transactions queue.Queue
	tx           *Transaction
}

func newTransactionContext(accounts accounts, tx *Transaction) *TransactionContext {
	ctx := &TransactionContext{
		accounts: accounts,
		balances: make(map[common.AccountID]uint64),
		stakes:   make(map[common.AccountID]uint64),

		contracts:        make(map[common.TransactionID][]byte),
		contractNumPages: make(map[common.TransactionID]uint64),
		contractPages:    make(map[common.TransactionID]map[uint64][]byte),

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

func (c *TransactionContext) ReadAccountBalance(id common.AccountID) (uint64, bool) {
	if balance, ok := c.balances[id]; ok {
		return balance, true
	}

	balance, exists := c.accounts.ReadAccountBalance(id)
	if exists {
		c.WriteAccountBalance(id, balance)
	}
	return balance, exists
}

func (c *TransactionContext) ReadAccountStake(id common.AccountID) (uint64, bool) {
	if stake, ok := c.stakes[id]; ok {
		return stake, true
	}

	stake, exists := c.accounts.ReadAccountStake(id)
	if exists {
		c.WriteAccountStake(id, stake)
	}
	return stake, exists
}

func (c *TransactionContext) ReadAccountContractCode(id common.TransactionID) ([]byte, bool) {
	if code, ok := c.contracts[id]; ok {
		return code, true
	}

	code, exists := c.accounts.ReadAccountContractCode(id)
	if exists {
		c.WriteAccountContractCode(id, code)
	}
	return code, exists
}

func (c *TransactionContext) ReadAccountContractNumPages(id common.AccountID) (uint64, bool) {
	if numPages, ok := c.contractNumPages[id]; ok {
		return numPages, true
	}

	numPages, exists := c.accounts.ReadAccountContractNumPages(id)
	if exists {
		c.WriteAccountContractNumPages(id, numPages)
	}
	return numPages, exists
}

func (c *TransactionContext) ReadAccountContractPage(id common.AccountID, idx uint64) ([]byte, bool) {
	if pages, ok := c.contractPages[id]; ok {
		if page, ok := pages[idx]; ok {
			return page, true
		}
	}

	page, exists := c.accounts.ReadAccountContractPage(id, idx)

	if exists {
		c.WriteAccountContractPage(id, idx, page)
	}

	return page, exists
}

func (c *TransactionContext) WriteAccountBalance(id common.AccountID, balance uint64) {
	c.balances[id] = balance
}

func (c *TransactionContext) WriteAccountStake(id common.AccountID, stake uint64) {
	c.stakes[id] = stake
}

func (c *TransactionContext) WriteAccountContractCode(id common.TransactionID, code []byte) {
	c.contracts[id] = code
}

func (c *TransactionContext) WriteAccountContractNumPages(id common.TransactionID, numPages uint64) {
	c.contractNumPages[id] = numPages
}

func (c *TransactionContext) WriteAccountContractPage(id common.TransactionID, idx uint64, page []byte) {
	pages, exist := c.contractPages[id]
	if !exist {
		pages = make(map[uint64][]byte)
		c.contractPages[id] = pages
	}

	pages[idx] = page
}

func (c *TransactionContext) apply(processors map[byte]TransactionProcessor) error {
	for c.transactions.Len() > 0 {
		c.tx = c.transactions.PopFront().(*Transaction)

		processor, exists := processors[c.tx.Tag]
		if !exists {
			return errors.Errorf("wavelet: transaction processor not registered for tag %d", c.tx.Tag)
		}

		err := processor(c)
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

	for id, code := range c.contracts {
		c.accounts.WriteAccountContractCode(id, code)
	}

	for id, numPages := range c.contractNumPages {
		c.accounts.WriteAccountContractNumPages(id, numPages)
	}

	for id, pages := range c.contractPages {
		for idx, page := range pages {
			c.accounts.WriteAccountContractPage(id, idx, page)
		}
	}

	return nil
}
