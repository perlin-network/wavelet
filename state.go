package wavelet

import (
	"bytes"
	"encoding/hex"
	"github.com/perlin-network/wavelet/events"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/params"

	"encoding/binary"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/wavelet/security"
	"github.com/pkg/errors"
	"sort"
)

var (
	BucketAccountRoots = []byte("acctlist_")
	BucketAccounts     = []byte("account_")
	BucketAccountIDs   = []byte("accountid_")
	BucketPreStates    = []byte("prestates_")
	BucketDeltas       = []byte("deltas_")
	//BucketContracts   = merge(BucketAccounts, ContractPrefix)
)

// pending represents a single transaction to be processed. A utility struct
// used for lexicographically-least topological sorting of a set of transactions.
type pending struct {
	tx    *database.Transaction
	depth uint64
}

type state struct {
	*Ledger

	processors [256]TransactionProcessor
}

func (s *state) RegisterTransactionProcessor(tag byte, p TransactionProcessor) {
	s.processors[int(tag)] = p
}

func (s *state) collectRewardedAncestors(tx *database.Transaction, filterOutSender []byte, depth int) ([]*database.Transaction, error) {
	if depth == 0 {
		return nil, nil
	}

	ret := make([]*database.Transaction, 0)

	parents := make([][]byte, len(tx.Parents))
	copy(parents, tx.Parents)
	sort.Slice(parents, func(i, j int) bool {
		return bytes.Compare(parents[i], parents[j]) == -1
	})

	for _, p := range parents {
		parent, err := s.Ledger.Store.GetBySymbol(p)
		if err != nil {
			return nil, err
		}

		if !bytes.Equal(parent.Sender, filterOutSender) {
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

func (s *state) randomlySelectValidator(tx *database.Transaction, amount uint64, depth int) ([]byte, error) {
	readStake := func(accountID []byte) uint64 {
		account := LoadAccount(s.Ledger.Accounts(), accountID)

		stake, _ := account.Load("stake")
		if stake == nil {
			return 0
		}

		return readUint64(stake)
	}

	if depth < 0 {
		return nil, errors.New("invalid depth")
	}

	candidateRewardees, err := s.collectRewardedAncestors(tx, tx.Sender, depth)
	if err != nil {
		return nil, err
	}

	if len(candidateRewardees) == 0 {
		return nil, nil
	}

	stakes := make([]uint64, len(candidateRewardees))
	totalStake := uint64(0)

	var buffer bytes.Buffer

	for _, rewardee := range candidateRewardees {
		buffer.Write(rewardee.Id)

		stake := readStake(rewardee.Sender)
		stakes = append(stakes, stake)
		totalStake += stake
	}

	entropy := security.Hash(buffer.Bytes())

	if totalStake == 0 {
		//log.Warn().Msg("None of the candidates have any stakes available. Skipping reward.")
		return nil, nil
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

func (s *state) ExecuteContract(txID []byte, entry string, param []byte) ([]byte, error) {
	account := LoadAccount(s.Ledger.Accounts(), txID)

	executor := NewContractExecutor(account, nil, param, ContractGasPolicy{nil, 100000})

	err := executor.Run(entry)
	if err != nil && errors.Cause(err) != ErrEntrypointNotFound {
		return nil, err
	}

	return executor.Result, nil
}

// applyTransaction runs a transaction, gets any transactions created by said transaction, and
// applies those transactions to the ledger state.
func (s *state) applyTransaction(tx *database.Transaction) error {
	s.Ledger.Store.Put(merge(BucketPreStates, []byte(tx.Id)), s.Ledger.Accounts().GetRoot())

	processor := s.processors[int(tx.Tag&0xff)]
	if processor == nil {
		return errors.New("no processor for incoming tx found")
	}

	ctx := newTransactionContext(s.Ledger, tx)
	err := ctx.run(processor)
	if err != nil {
		return err
	}

	go events.Publish(nil, &events.TransactionAppliedEvent{ID: tx.Id})

	return nil
}

// NumTransactions returns the number of transactions in the ledger.
func (s *state) NumTransactions() uint64 {
	return s.Size(database.BucketTx)
}

// NumContracts returns the number of smart contracts in the ledger.
func (s *state) NumContracts() uint64 {
	return 0
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
		acct := LoadAccount(s.Ledger.Accounts(), publicKey)

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
func (s *state) LoadContract(txID []byte) ([]byte, error) {
	account := LoadAccount(s.Ledger.Accounts(), txID)

	contractCode, ok := account.Load(params.KeyContractCode)
	if !ok {
		return nil, errors.Errorf("contract ID %s has no code", hex.EncodeToString(txID))
	}
	return contractCode, nil
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
				account := LoadAccount(writeBytes(ContractID(string(txID))))
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
