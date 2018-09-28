package wavelet

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"time"
)

var (
	BucketAccounts = []byte("account_")
	BucketApplied  = []byte("tx_applied_")
)

type state struct {
	*Ledger
}

func (s *state) updateLedgerStateLoop() {
	timer := time.NewTicker(1000 * time.Millisecond)

	for {
		select {
		case <-s.kill:
			break
		case <-timer.C:
			s.updateLedgerState()
		}
	}

	timer.Stop()
}

func (s *state) updateLedgerState() {
	numApplied, numAccepted := s.Size(BucketApplied), s.Size(BucketAcceptedIndex)

	var appliedList []string

	// Apply all transactions not yet applied.
	for i := numApplied; i < numAccepted; i++ {
		tx, err := s.GetAcceptedByIndex(i)
		if err != nil {
			return
		}

		_, err = s.NextSequence(BucketApplied)
		if err != nil {
			return
		}

		err = s.Put(merge(BucketApplied, writeBytes(tx.Id)), []byte{0})
		if err != nil {
			return
		}

		appliedList = append(appliedList, tx.Id)
	}

	if len(appliedList) > 0 {
		// Trim transaction IDs.
		for i := 0; i < len(appliedList); i++ {
			appliedList[i] = appliedList[i][:10]
		}

		log.Info().Interface("applied", appliedList).Msgf("Applied %d transactions.", len(appliedList))
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
