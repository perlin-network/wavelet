package wavelet

import "github.com/pkg/errors"

var (
	BucketAccounts = []byte("account_")
)

type state struct {
	*Ledger
}

// LoadAccount reads the account data for a given hex public key.
func (s *state) LoadAccount(key string) (*Account, error) {
	bytes, err := s.Get(merge(BucketAccounts, writeBytes(key)))
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
	err := s.Put(merge(BucketAccounts, writeBytes(account.PublicKey)), account.MarshalBinary())
	if err != nil {
		return err
	}

	// TODO: Report deltas.

	return nil
}
