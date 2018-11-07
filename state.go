package wavelet

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/perlin-network/wavelet/events"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/params"

	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/life/exec"

	"encoding/binary"
	"github.com/perlin-network/wavelet/security"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"sort"
)

var (
	BucketAccounts  = []byte("account_")
	BucketDeltas    = []byte("deltas_")
	BucketContracts = merge(BucketAccounts, ContractPrefix)
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

func (s *state) collectRewardedAncestors(tx *database.Transaction, filterOutSender string, depth int) ([]*database.Transaction, error) {
	if depth == 0 {
		return nil, nil
	}

	ret := make([]*database.Transaction, 0)

	parents := make([]string, len(tx.Parents))
	copy(parents, tx.Parents)
	sort.Strings(parents)

	for _, p := range parents {
		parent, err := s.Ledger.Store.GetBySymbol(p)
		if err != nil {
			return nil, err
		}

		if parent.Sender != filterOutSender {
			ret = append(ret, parent)
		}

		parentAncestors, err := s.collectRewardedAncestors(parent, filterOutSender, depth-1)
		if err != nil {
			return nil, err
		}
		ret = append(ret, parentAncestors...)
	}

	return ret, nil
}

func (s *state) randomlySelectValidator(tx *database.Transaction, amount uint64, depth int) (string, error) {
	readStake := func(sAccountID string) uint64 {
		accountID, err := hex.DecodeString(sAccountID)
		if err != nil {
			panic(err) // shouldn't happen?
		}

		account, err := s.LoadAccount(accountID)
		if err != nil {
			return 0
		}

		_, stake := account.State.Load("stake")
		if stake == nil {
			return 0
		}

		return readUint64(stake)
	}

	if depth < 0 {
		return "", errors.New("invalid depth")
	}

	candidateRewardees, err := s.collectRewardedAncestors(tx, tx.Sender, depth)
	if err != nil {
		return "", err
	}

	if len(candidateRewardees) == 0 {
		return "", nil
	}

	stakes := make([]uint64, len(candidateRewardees))
	totalStake := uint64(0)

	var buffer bytes.Buffer

	for _, rewardee := range candidateRewardees {
		buffer.WriteString(rewardee.Id)

		stake := readStake(rewardee.Sender)
		stakes = append(stakes, stake)
		totalStake += stake
	}

	entropy := security.Hash(buffer.Bytes())

	if totalStake == 0 {
		//log.Warn().Msg("None of the candidates have any stakes available. Skipping reward.")
		return "", nil
	}

	threshold := float64(binary.LittleEndian.Uint64(entropy)%uint64(0xffff)) / float64(0xffff)
	acc := float64(0.0)

	var rewarded *database.Transaction

	for i, tx := range candidateRewardees {
		acc += float64(stakes[i]) / float64(totalStake)

		if acc >= threshold {
			log.Debug().Msgf("Selected validator rewardee transaction %s (%f/%f).", tx.Sender, acc, threshold)
			rewarded = tx
			break
		}
	}

	// If there is no selected transaction that deserves a reward, give the reward to the last reward candidate.
	if rewarded == nil {
		rewarded = candidateRewardees[len(candidateRewardees)-1]
	}

	return rewarded.Sender, nil
}

// applyTransaction runs a transaction, gets any transactions created by said transaction, and
// applies those transactions to the ledger state.
func (s *state) applyTransaction(tx *database.Transaction) error {
	accountDeltas := &Deltas{Deltas: make(map[string]*Deltas_List)}

	rewardee, err := s.randomlySelectValidator(tx, params.ValidatorRewardAmount, params.ValidatorRewardDepth)
	if err != nil {
		return err
	}

	pending := queue.New()
	pending.PushBack(tx)

	// Returning from within this loop is not allowed.
	for pending.Len() > 0 {
		tx := pending.PopFront().(*database.Transaction)

		senderID, err := hex.DecodeString(tx.Sender)
		if err != nil {
			log.Error().Err(err).Str("tx", tx.Id).Msg("Failed to decode sender ID.")
			continue
		}

		accounts := make(map[string]*Account)

		sender, exists := accounts[writeString(senderID)]

		if !exists {
			sender, err = s.LoadAccount(senderID)

			if err != nil {
				if tx.Nonce == 0 {
					sender = NewAccount(senderID)
				} else {
					log.Error().Err(err).Str("tx", tx.Id).Msgf("Sender account %s does not exist.", tx.Sender)
					continue
				}
			}

			accounts[writeString(senderID)] = sender
		}

		if tx.Tag == params.TagNop {
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

		var initialBalance uint64

		initialBalanceBytes, ok := sender.Load("balance")

		if ok && len(initialBalanceBytes) > 0 {
			initialBalance = readUint64(initialBalanceBytes)
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
		sender.Nonce++

		var finalBalance uint64

		finalBalanceBytes, ok := sender.Load("balance")

		if ok && len(finalBalanceBytes) > 0 {
			finalBalance = readUint64(finalBalanceBytes)
		}

		if len(rewardee) > 0 {
			deducted := (initialBalance - finalBalance) * uint64(params.TransactionFeePercentage) / 100

			// Bare minimum transaction fee that must be paid by all transactions.
			if deducted < params.ValidatorRewardAmount {
				deducted = params.ValidatorRewardAmount
			}

			if finalBalance < deducted {
				deducted = finalBalance
			}

			rewardeeID, err := hex.DecodeString(rewardee)
			if err != nil {
				panic(err) // Should not happen.
			}

			rewardeeAccount, ok := accounts[writeString(rewardeeID)]

			if !ok {
				rewardeeAccount, err = s.LoadAccount(rewardeeID)
				if err != nil {
					rewardeeAccount = NewAccount(rewardeeID)
				}
				accounts[writeString(rewardeeID)] = rewardeeAccount
			}

			var rewardeeBalance uint64

			rewardeeBalanceBytes, ok := rewardeeAccount.Load("balance")

			if ok && len(rewardeeBalanceBytes) > 0 {
				rewardeeBalance = readUint64(rewardeeBalanceBytes)
			}

			sender.Store("balance", writeUint64(finalBalance-deducted))
			rewardeeAccount.Store("balance", writeUint64(rewardeeBalance+deducted))

			if _, exists := accountDeltas.Deltas[writeString(sender.PublicKey)]; !exists {
				accountDeltas.Deltas[writeString(sender.PublicKey)] = new(Deltas_List)
			}

			accountDeltas.Deltas[writeString(sender.PublicKey)].List = append(accountDeltas.Deltas[writeString(sender.PublicKey)].List, &Delta{
				Account:  sender.PublicKey,
				Key:      "balance",
				OldValue: finalBalanceBytes,
				NewValue: writeUint64(finalBalance - deducted),
			})

			if _, exists := accountDeltas.Deltas[writeString(rewardeeAccount.PublicKey)]; !exists {
				accountDeltas.Deltas[writeString(rewardeeAccount.PublicKey)] = new(Deltas_List)
			}

			accountDeltas.Deltas[writeString(rewardeeAccount.PublicKey)].List = append(accountDeltas.Deltas[writeString(rewardeeAccount.PublicKey)].List, &Delta{
				Account:  rewardeeAccount.PublicKey,
				Key:      "balance",
				OldValue: rewardeeBalanceBytes,
				NewValue: writeUint64(rewardeeBalance + deducted),
			})

			log.Debug().Msgf("Transaction fee of %d PERLs transferred from %s to validator %s as a reward.", deducted, sender.PublicKeyHex(), rewardee)
		}

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
		if tx.Tag == params.TagCreateContract {
			// load the contract code into the tx before running the service
			contractCode, err := s.LoadContract(tx.Id)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "Unable to load contract for txID %s", tx.Id)
			}
			tx.Payload = contractCode
		}

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
		// TODO: need a better way to get counts
		if bytes.HasPrefix(account.PublicKey, ContractPrefix) {
			_, err = s.NextSequence(BucketContracts)
			if err != nil {
				return err
			}
		} else {
			_, err = s.NextSequence(BucketAccounts)
			if err != nil {
				return err
			}
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

// NumAccounts returns the number of accounts recorded in the ledger.
func (s *state) NumAccounts() uint64 {
	return s.Size(BucketAccounts)
}

// NumTransactions returns the number of transactions in the ledger.
func (s *state) NumTransactions() uint64 {
	return s.Size(database.BucketTx)
}

// NumContracts returns the number of smart contracts in the ledger.
func (s *state) NumContracts() uint64 {
	return s.Size(BucketContracts)
}

// NumAcceptedTransactions returns the number of accepted transactions in the ledger.
func (s *state) NumAcceptedTransactions() uint64 {
	return s.Size(BucketAcceptedIndex)
}

// PaginateTransactions returns a page of transactions ordered by insertion starting from the offset index.
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
			if k == params.KeyContractCode {
				v = writeBytes("<code here>")
			}
			data.State[k] = v
		})

		json[hex.EncodeToString(publicKey)] = data

		return nil
	})

	return json
}

// LoadContract loads a smart contract from the database given its tx id.
// The key in the database will be of the form "account_C-txID"
func (s *state) LoadContract(txID string) ([]byte, error) {
	contractKey := merge(BucketContracts, writeBytes(txID))
	bytes, err := s.Get(contractKey)
	if err != nil {
		return nil, errors.Wrapf(err, "contract ID %s not found in ledger state", txID)
	}

	account := NewAccount(writeBytes(ContractID(txID)))
	err = account.Unmarshal(bytes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode account")
	}

	contractCode, ok := account.Load(params.KeyContractCode)
	if !ok {
		return nil, errors.Errorf("contract ID %s has no contract code", txID)
	}
	return contractCode, nil
}

// SaveContract saves a smart contract to the database given its tx id.
// The key in the database will be of the form "account_C-txID"
func (s *state) SaveContract(txID string, contractCode []byte) error {
	contractKey := merge(BucketContracts, writeBytes(txID))

	account := NewAccount(writeBytes(ContractID(txID)))

	if bytes, err := s.Get(contractKey); err == nil {
		if err := account.Unmarshal(bytes); err != nil {
			return errors.Wrapf(err, "failed to decode account")
		}
		account.Nonce++
	}

	account.Store(params.KeyContractCode, contractCode)

	if err := s.SaveAccount(account, nil); err != nil {
		return err
	}

	return nil
}

// PaginateContracts paginates through the smart contracts found in the ledger by searching for the prefix
// "account_C-*" in the database.
func (s *state) PaginateContracts(offset, pageSize uint64) []*Contract {
	size := s.NumContracts()

	if offset > size || pageSize == 0 {
		return nil
	}

	if offset+pageSize > size {
		pageSize = size - offset
	}

	var page []*Contract

	i := uint64(0)

	s.Store.ForEach(BucketContracts, func(txID []byte, encoded []byte) error {
		if i >= offset && uint64(len(page)) < pageSize {
			account := NewAccount(writeBytes(ContractID(string(txID))))
			err := account.Unmarshal(encoded)
			if err != nil {
				err := errors.Wrapf(err, "failed to decode contract bytes")
				log.Error().Err(err).Msg("")
				return err
			}

			contractCode, ok := account.Load(params.KeyContractCode)
			if !ok {
				err := errors.Errorf("contract ID %s has no contract code", txID)
				log.Error().Err(err).Msg("")
				return err
			}

			contract := &Contract{
				TransactionID: string(txID),
				Code:          contractCode,
			}

			page = append(page, contract)
		}

		i++

		return nil
	})

	return page
}
