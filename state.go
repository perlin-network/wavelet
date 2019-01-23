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
	BucketAccountList = []byte("acctlist_")
	BucketAccounts    = []byte("account_")
	BucketAccountIDs  = []byte("accountid_")
	BucketPreStates   = []byte("prestates_")
	BucketDeltas      = []byte("deltas_")
	BucketContracts   = merge(BucketAccounts, ContractPrefix)
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

		account := NewAccount(s.Ledger, accountID)

		stake, _ := account.Load("stake")
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

func (s *state) ExecuteContract(txID string, entry string, param []byte) ([]byte, error) {
	account := NewAccount(s.Ledger, writeBytes(txID))

	code, ok := account.Load(params.KeyContractCode)
	if !ok {
		return nil, errors.Errorf("contract ID %s has no contract code", txID)
	}

	var result []byte

	executor := &ContractExecutor{
		GasTable: nil,
		GasLimit: 100000, // TODO: Set the limit properly
		Code:     code,
		QueueTransaction: func(tag string, payload []byte) {
			panic("not supported in local execution mode")
		},
		GetActivationReasonLen: func() int {
			return len(param)
		},
		GetActivationReason: func() []byte {
			return param
		},
		GetDataItemLen: func(key string) int {
			val, _ := account.Load(string(merge(ContractCustomStatePrefix, writeBytes(key))))
			return len(val)
		},
		GetDataItem: func(key string) []byte {
			val, _ := account.Load(string(merge(ContractCustomStatePrefix, writeBytes(key))))
			return val
		},
		SetDataItem: func(key string, val []byte) {
			if key == ".local_result" {
				result = make([]byte, len(val))
				copy(result, val)
			} else {
				panic("not supported in local execution mode")
			}
		},
	}
	err := executor.RunLocal(entry)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// applyTransaction runs a transaction, gets any transactions created by said transaction, and
// applies those transactions to the ledger state.
func (s *state) applyTransaction(tx *database.Transaction) error {
	rewardee, err := s.randomlySelectValidator(tx, params.ValidatorRewardAmount, params.ValidatorRewardDepth)
	if err != nil {
		return err
	}

	s.Ledger.Store.Put(merge(BucketPreStates, []byte(tx.Id)), s.Ledger.accountList.GetRoot())

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
			if x, _ := s.Store.Get(merge(BucketAccountIDs, senderID)); x == nil && tx.Nonce != 0 {
				log.Error().Err(err).Str("tx", tx.Id).Msgf("Sender account %s does not exist.", tx.Sender)
				continue
			}

			sender = NewAccount(s.Ledger, senderID)
			accounts[writeString(senderID)] = sender
		}

		if tx.Tag == params.TagNop {
			sender.SetNonce(sender.GetNonce() + 1)
			continue
		}

		newlyPending, err := s.doApplyTransaction(tx, accounts)
		if err != nil {
			log.Warn().Interface("tx", tx).Err(err).Msg("Transaction failed to get applied to the ledger state.")
		}

		initialBalance := sender.GetBalance()
		finalBalance := sender.GetBalance()

		// Increment the senders account nonce.
		sender.SetNonce(sender.GetNonce() + 1)

		if len(rewardee) > 0 {
			var deducted uint64

			if finalBalance < initialBalance {
				deducted = (initialBalance - finalBalance) * uint64(params.TransactionFeePercentage) / 100
			}

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
				rewardeeAccount := NewAccount(s.Ledger, rewardeeID)
				accounts[writeString(rewardeeID)] = rewardeeAccount
			}

			var rewardeeBalance uint64

			rewardeeBalanceBytes, ok := rewardeeAccount.Load("balance")

			if ok && len(rewardeeBalanceBytes) > 0 {
				rewardeeBalance = readUint64(rewardeeBalanceBytes)
			}

			sender.Store("balance", writeUint64(finalBalance-deducted))
			rewardeeAccount.Store("balance", writeUint64(rewardeeBalance+deducted))

			sender.SetBalance(finalBalance - deducted)
			rewardeeAccount.SetBalance(rewardeeBalance + deducted)

			log.Debug().Msgf("Transaction fee of %d PERLs transferred from %s to validator %s as a reward.", deducted, sender.PublicKeyHex(), rewardee)
		}

		for _, tx := range newlyPending {
			pending.PushBack(tx)
		}

		// Save all modified accounts to the ledger.
		for _, account := range accounts {
			account.Writeback()
			log.Debug().
				Uint64("nonce", account.GetNonce()).
				Str("public_key", hex.EncodeToString(account.PublicKey())).
				Str("tx", tx.Id).
				Msg("Applied transaction.")
		}
	}

	go events.Publish(nil, &events.TransactionAppliedEvent{ID: tx.Id})

	return nil
}

// doApplyTransaction runs a transaction through a transaction processor and applies its recorded
// changes to the ledger state.
//
// Any additional transactions that are recursively generated by smart contracts for example are returned.
func (s *state) doApplyTransaction(tx *database.Transaction, accounts map[string]*Account) ([]*database.Transaction, error) {
	// Iterate through all registered services and run them on the transactions given their tags and payload.
	var pendingTransactions []*database.Transaction

	for _, service := range s.services {
		// TODO
		/*
			if tx.Tag == params.TagCreateContract {
				// load the contract code into the tx before running the service
				contractCode, err := s.LoadContract(tx.Id)
				if err != nil {
					return nil, nil, errors.Wrapf(err, "Unable to load contract for txID %s", tx.Id)
				}
				tx.Payload = contractCode
			}
		*/

		pending, err := service.Run(tx, accounts)

		if err != nil {
			return nil, err
		}

		if len(pending) > 0 {
			pendingTransactions = append(pendingTransactions, pending...)
		}
	}

	return pendingTransactions, nil
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

	s.Store.ForEach(BucketAccountIDs, func(publicKey, _ []byte) error {
		acct := NewAccount(s.Ledger, publicKey)

		data := accountData{Nonce: acct.GetNonce(), State: make(map[string][]byte)}

		acct.Range(func(k string, v []byte) {
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
	account := NewAccount(s.Ledger, writeBytes(ContractID(txID)))

	contractCode, ok := account.Load(params.KeyContractCode)
	if !ok {
		return nil, errors.Errorf("contract ID %s has no contract code", txID)
	}
	return contractCode, nil
}

// SaveContract saves a smart contract to the database given its tx id.
// The key in the database will be of the form "account_C-txID"
func (s *state) SaveContract(txID string, contractCode []byte) error {
	account := NewAccount(s.Ledger, writeBytes(ContractID(txID)))
	account.Store(params.KeyContractCode, contractCode)
	account.Writeback()

	return nil
}

// PaginateContracts paginates through the smart contracts found in the ledger by searching for the prefix
// "account_C-*" in the database.
func (s *state) PaginateContracts(offset, pageSize uint64) []*Contract {
	return nil
	/*
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
	*/
}
