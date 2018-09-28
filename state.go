package wavelet

import (
	"encoding/hex"
	"github.com/pkg/errors"
)

var (
	BucketAccounts = []byte("account_")
	BucketApplied  = []byte("tx_applied_")
)

type state struct {
	*Ledger
}

func (s *state) applyTransaction(symbol string) {
	tx, err := s.GetBySymbol(symbol)
	if err != nil {
		return
	}

	sender, err := hex.DecodeString(tx.Sender)
	if err != nil {
		return
	}

	_, err = s.state.LoadAccount(sender)
	if err != nil {
		if tx.Nonce == 0 {
			//account = NewAccount(sender)
		} else {
			return
		}
	}

	// TODO: Apply transaction to ledger state here.

	//account.Nonce = tx.Nonce + 1
	//
	//err = s.SaveAccount(account, nil)
	//if err != nil {
	//	return
	//}

	_, err = s.NextSequence(BucketApplied)
	if err != nil {
		return
	}

	err = s.Put(merge(BucketApplied, writeBytes(symbol)), []byte{0})
	if err != nil {
		return
	}
}

// LoadAccount reads the account data for a given hex public key.
func (s *state) LoadAccount(key []byte) (*Account, error) {
	bytes, err := s.Get(merge(BucketAccounts, key))
	if err != nil {
		return nil, errors.Wrapf(err, "account %s not found in ledger state", key)
	}

	account := NewAccount(key)
	err = account.Unmarshal(bytes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode account bytes")
	}

	return account.Clone(), nil
}

func (s *state) SaveAccount(account *Account, deltas []*Delta) error {
	err := s.Put(merge(BucketAccounts, account.PublicKey), account.MarshalBinary())
	if err != nil {
		return err
	}

	// TODO: Report deltas.

	return nil
}
