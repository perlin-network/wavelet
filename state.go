package wavelet

import (
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/life/exec"
	"github.com/perlin-network/wavelet/events"
	"github.com/perlin-network/wavelet/log"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"io/ioutil"
	"path/filepath"
	"strings"
)

var (
	BucketAccounts = []byte("account_")
	BucketDeltas   = []byte("deltas_")
)

// pending represents a single transaction to be processed. A utility struct
// used for lexicographically-least topological sorting of a set of transactions.
type pending struct {
	tx    *database.Transaction
	depth uint64
}

type state struct {
	*Ledger

	services []*service
}

// registerServicePath registers all the services in a path.
func (s *state) registerServicePath(path string) error {
	files, err := filepath.Glob(fmt.Sprintf("%s/*.wasm", path))
	if err != nil {
		return err
	}

	for _, f := range files {
		name := filepath.Base(f)

		if err := s.registerService(name[:len(name)-5], f); err != nil {
			return err
		}
		log.Info().Str("module", name).Msg("Registered transaction processor service.")
	}

	if len(s.services) == 0 {
		return errors.Errorf("No WebAssembly services were successfully registered for path: %s", path)
	}

	return nil
}

// registerService internally loads a *.wasm module representing a service, and registers the service
// with a specified name.
//
// Warning: will panic should there be errors in loading the service.
func (s *state) registerService(name string, path string) error {
	if !strings.HasSuffix(path, ".wasm") {
		return errors.Errorf("service code %s file should be in *.wasm format", path)
	}

	code, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	service := NewService(s, name)

	service.vm, err = exec.NewVirtualMachine(code, exec.VMConfig{
		DefaultMemoryPages: 128,
		DefaultTableSize:   65536,
	}, service, nil)

	if err != nil {
		return err
	}

	var exists bool

	service.entry, exists = service.vm.GetFunctionExport("process")
	if !exists {
		return errors.Errorf("could not find 'process' func in %s *.wasm file", path)
	}

	s.services = append(s.services, service)

	return nil
}

// applyTransaction runs a transaction, gets any transactions created by said transaction, and
// applies those transactions to the ledger state.
func (s *state) applyTransaction(tx *database.Transaction) error {
	accountDeltas := &Deltas{Deltas: make(map[string]*Deltas_List)}

	pending := queue.New()
	pending.PushBack(tx)

	for pending.Len() > 0 {
		tx := pending.PopFront().(*database.Transaction)

		senderID, err := hex.DecodeString(tx.Sender)
		if err != nil {
			return err
		}

		accounts := make(map[string]*Account)

		sender, exists := accounts[writeString(senderID)]

		if !exists {
			sender, err = s.LoadAccount(senderID)

			if err != nil {
				if tx.Nonce == 0 {
					sender = NewAccount(senderID)
				} else {
					return errors.Wrapf(err, "sender account %s does not exist", tx.Sender)
				}
			}

			accounts[writeString(senderID)] = sender
		}

		if tx.Tag == "nop" {
			sender.Nonce++

			for id, account := range accounts {
				log.Debug().
					Uint64("nonce", account.Nonce).
					Str("public_key", hex.EncodeToString(account.PublicKey)).
					Str("tx", tx.Id).
					Msg("Applied transaction.")

				list, available := accountDeltas.Deltas[id]
				if available {
					s.SaveAccount(account, list.List)
				} else {
					s.SaveAccount(account, nil)
				}
			}

			continue
		}

		deltas, newlyPending, err := s.doApplyTransaction(tx)
		if err != nil {
			log.Warn().Interface("tx", tx).Err(err).Msg("Transaction failed to get applied to the ledger state.")
		}

		if err == nil {
			for _, delta := range deltas {
				accountID := writeString(delta.Account)

				account, exists := accounts[accountID]

				if !exists {
					account, err = s.LoadAccount(delta.Account)
					if err != nil {
						account = NewAccount(delta.Account)
					}

					accounts[accountID] = account
				}

				delta.OldValue, _ = account.Load(delta.Key)
				account.Store(delta.Key, delta.NewValue)

				if _, exists := accountDeltas.Deltas[accountID]; !exists {
					accountDeltas.Deltas[accountID] = new(Deltas_List)
				}

				accountDeltas.Deltas[accountID].List = append(accountDeltas.Deltas[accountID].List, delta)
			}
		}

		// Increment the senders account nonce.
		accounts[writeString(sender.PublicKey)].Nonce++

		for _, tx := range newlyPending {
			pending.PushBack(tx)
		}

		// Save all modified accounts to the ledger.
		for id, account := range accounts {
			log.Debug().
				Uint64("nonce", account.Nonce).
				Str("public_key", hex.EncodeToString(account.PublicKey)).
				Str("tx", tx.Id).
				Msg("Applied transaction.")

			list, available := accountDeltas.Deltas[id]
			if available {
				s.SaveAccount(account, list.List)
			} else {
				s.SaveAccount(account, nil)
			}
		}
	}

	bytes, err := accountDeltas.Marshal()
	if err != nil {
		return err
	}

	err = s.Put(merge(BucketDeltas, writeBytes(tx.Id)), bytes)
	if err != nil {
		return err
	}

	go events.Publish(nil, &events.TransactionAppliedEvent{ID: tx.Id})

	return nil
}

// doApplyTransaction runs a transaction through a transaction processor and applies its recorded
// changes to the ledger state.
//
// Any additional transactions that are recursively generated by smart contracts for example are returned.
func (s *state) doApplyTransaction(tx *database.Transaction) ([]*Delta, []*database.Transaction, error) {
	var deltas []*Delta

	// Iterate through all registered services and run them on the transactions given their tags and payload.
	var pendingTransactions []*database.Transaction

	for _, service := range s.services {
		new, pending, err := service.Run(tx)

		if err != nil {
			return nil, nil, err
		}

		deltas = append(deltas, new...)

		if len(pending) > 0 {
			pendingTransactions = append(pendingTransactions, pending...)
		}
	}

	return deltas, pendingTransactions, nil
}

func (s *state) doRevertTransaction(pendingList *[]pending) {
	accountDeltas := new(Deltas)
	accounts := make(map[string]*Account)

	for _, pending := range *pendingList {
		deltaListKey := merge(BucketDeltas, writeBytes(pending.tx.Id))
		deltaListBytes, err := s.Get(deltaListKey)

		// Revert deltas only if the transaction has a list of deltas available.
		if err == nil {
			err = accountDeltas.Unmarshal(deltaListBytes)

			if err != nil {
				continue
			}

			for accountID, list := range accountDeltas.Deltas {
				// Go from the end of the deltas list and apply the old value.
				for i := len(list.List) - 1; i >= 0; i-- {
					account, exists := accounts[accountID]
					if !exists {
						account, err = s.LoadAccount(writeBytes(accountID))

						if err != nil {
							continue
						}

						accounts[accountID] = account
					}

					if list.List[i].OldValue == nil {
						account.State.Delete(list.List[i].Key)
					} else {
						account.Store(list.List[i].Key, list.List[i].OldValue)
					}
				}
			}

			// Delete delta list after reverting the transaction.
			s.Delete(deltaListKey)
		}

		senderID, err := hex.DecodeString(pending.tx.Sender)
		if err != nil {
			continue
		}

		sender, exists := accounts[writeString(senderID)]
		if !exists {
			sender, err = s.LoadAccount(senderID)

			if err != nil {
				continue
			}

			accounts[writeString(senderID)] = sender
		}

		sender.Nonce = pending.tx.Nonce
	}

	// Save changes to all accounts.
	for id, account := range accounts {
		log.Debug().
			Uint64("nonce", account.Nonce).
			Str("public_key", hex.EncodeToString(account.PublicKey)).
			Msg("Reverted transaction.")

		list, available := accountDeltas.Deltas[id]
		if available {
			s.SaveAccount(account, list.List)
		} else {
			s.SaveAccount(account, nil)
		}
	}
}

// LoadAccount loads an account from the database given its public key.
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

// SaveAccount saves an account to the database.
func (s *state) SaveAccount(account *Account, deltas []*Delta) error {
	accountKey := merge(BucketAccounts, account.PublicKey)

	existed, err := s.Has(accountKey)
	if err != nil {
		return err
	}

	// If this is a new account, increment the bucket size by 1.
	if !existed {
		_, err = s.NextSequence(BucketAccounts)
		if err != nil {
			return err
		}
	}

	err = s.Put(accountKey, account.MarshalBinary())
	if err != nil {
		return err
	}

	updates := make(map[string][]byte)

	for _, delta := range deltas {
		updates[delta.Key] = delta.NewValue
	}

	go events.Publish(nil, &events.AccountUpdateEvent{
		Account: account.PublicKeyHex(),
		Nonce:   account.Nonce,
		Updates: updates,
	})

	return nil
}

// NumTransactions represents the number of transactions in the ledger.
func (s *state) NumTransactions() uint64 {
	return s.Size(database.BucketTx)
}

// NumAcceptedTransactions represents the number of accepted transactions in the ledger.
func (s *state) NumAcceptedTransactions() uint64 {
	return s.Size(BucketAcceptedIndex)
}

// Paginate returns a page of transactions ordered by insertion starting from the offset index.
func (s *state) PaginateTransactions(offset, pageSize uint64) []*database.Transaction {
	size := s.NumAcceptedTransactions()

	if offset > size || pageSize == 0 {
		return nil
	}

	if offset+pageSize > size {
		pageSize = size - offset
	}

	var page []*database.Transaction

	for i := offset; i < offset+pageSize; i++ {
		tx, err := s.GetAcceptedByIndex(i)
		if err != nil {
			continue
		}

		page = append(page, tx)
	}

	return page
}

// WasApplied returns whether or not a transaction was applied into the ledger.
func (s *state) WasApplied(symbol string) bool {
	applied, _ := s.Has(merge(BucketDeltas, writeBytes(symbol)))
	return applied
}

// Snapshot creates a snapshot of the current ledger state.
func (s *state) Snapshot() map[string]interface{} {
	type accountData struct {
		Nonce uint64
		State map[string][]byte
	}

	json := make(map[string]interface{})

	account := new(Account)

	s.Store.ForEach(BucketAccounts, func(publicKey, encoded []byte) error {
		err := account.Unmarshal(encoded)

		if err != nil {
			return err
		}

		data := accountData{Nonce: account.Nonce, State: make(map[string][]byte)}

		account.Range(func(k string, v []byte) {
			if k == "contract_code" {
				v = writeBytes("<code here>")
			}
			data.State[k] = v
		})

		json[hex.EncodeToString(publicKey)] = data

		return nil
	})

	return json
}
