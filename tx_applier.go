// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package wavelet

import (
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

type ContractExecutorState struct {
	Sender   AccountID
	GasLimit uint64
}

func ApplyTransferTransaction(snapshot *avl.Tree, round *Round, tx *Transaction, state *ContractExecutorState) (*avl.Tree, error) {
	params, err := ParseTransferTransaction(tx.Payload)
	if err != nil {
		return nil, err
	}

	code, codeAvailable := ReadAccountContractCode(snapshot, params.Recipient)

	if !codeAvailable && (params.GasLimit > 0 || len(params.FuncName) > 0 || len(params.FuncParams) > 0) {
		return nil, errors.New("transfer: transactions to non-contract accounts should not specify gas limit or function names or params")
	}

	senderBalance, _ := ReadAccountBalance(snapshot, tx.Creator)

	// FIXME(kenta): FOR TESTNET ONLY. FAUCET DOES NOT GET ANY PERLs DEDUCTED.
	if hex.EncodeToString(tx.Creator[:]) == sys.FaucetAddress {
		recipientBalance, _ := ReadAccountBalance(snapshot, params.Recipient)
		WriteAccountBalance(snapshot, params.Recipient, recipientBalance+params.Amount)

		return snapshot, nil
	}

	if senderBalance < params.Amount {
		return nil, errors.Errorf("transfer: %x tried send %d PERLs to %x, but only has %d PERLs",
			tx.Creator, params.Amount, params.Recipient, senderBalance)
	}

	if !codeAvailable {
		WriteAccountBalance(snapshot, tx.Creator, senderBalance-params.Amount)

		recipientBalance, _ := ReadAccountBalance(snapshot, params.Recipient)
		WriteAccountBalance(snapshot, params.Recipient, recipientBalance+params.Amount)

		return snapshot, nil
	}

	sender := tx.Creator
	if state != nil {
		sender = state.Sender
		params.GasLimit = state.GasLimit
	}

	if params.GasLimit == 0 {
		return nil, errors.New("transfer: gas limit for invoking smart contract must be greater than zero")
	}

	senderBalance, _ = ReadAccountBalance(snapshot, sender)

	if senderBalance < params.GasLimit {
		return nil, errors.Errorf("transfer: %x attempted to claim a gas limit of %d PERLs, but only has %d PERLs",
			sender, params.GasLimit, senderBalance)
	}

	WriteAccountBalance(snapshot, tx.Creator, senderBalance-params.Amount)

	recipientBalance, _ := ReadAccountBalance(snapshot, params.Recipient)
	WriteAccountBalance(snapshot, params.Recipient, recipientBalance+params.Amount)

	executor := &ContractExecutor{}

	if err := executor.Execute(snapshot, params.Recipient, round, tx, params.Amount, params.GasLimit, string(params.FuncName), params.FuncParams, code); err != nil {
		return nil, errors.Wrap(err, "transfer: failed to invoke smart contract")
	}

	if executor.GasLimitExceeded { // Revert changes and have the sender pay gas fees.
		WriteAccountBalance(snapshot, tx.Creator, senderBalance-executor.Gas)

		recipientBalance, _ := ReadAccountBalance(snapshot, params.Recipient)
		WriteAccountBalance(snapshot, params.Recipient, recipientBalance)

		logger := log.Contracts("gas")
		logger.Info().
			Hex("sender_id", tx.Creator[:]).
			Hex("contract_id", params.Recipient[:]).
			Uint64("gas", executor.Gas).
			Uint64("gas_limit", params.GasLimit).
			Msg("Exceeded gas limit while invoking smart contract function.")
	} else {
		WriteAccountBalance(snapshot, tx.Creator, senderBalance-params.Amount-executor.Gas)

		logger := log.Contracts("gas")
		logger.Info().
			Hex("sender_id", tx.Creator[:]).
			Hex("contract_id", params.Recipient[:]).
			Uint64("gas", executor.Gas).
			Uint64("gas_limit", params.GasLimit).
			Msg("Deducted PERLs for invoking smart contract function.")

		if state == nil {
			state = &ContractExecutorState{Sender: tx.Sender}
		}

		if params.GasLimit > executor.Gas {
			state.GasLimit = params.GasLimit - executor.Gas
		}

		for _, entry := range executor.Queue {
			switch entry.Tag {
			case sys.TagNop:
			case sys.TagTransfer:
				if _, err := ApplyTransferTransaction(snapshot, round, entry, state); err != nil {
					return nil, err
				}
			case sys.TagStake:
				if _, err := ApplyStakeTransaction(snapshot, round, entry); err != nil {
					return nil, err
				}
			case sys.TagContract:
				if _, err := ApplyContractTransaction(snapshot, round, entry, state); err != nil {
					return nil, err
				}
			case sys.TagBatch:
				if _, err := ApplyBatchTransaction(snapshot, round, entry); err != nil {
					return nil, err
				}
			}
		}
	}

	return snapshot, nil
}

func ApplyStakeTransaction(snapshot *avl.Tree, round *Round, tx *Transaction) (*avl.Tree, error) {
	params, err := ParseStakeTransaction(tx.Payload)
	if err != nil {
		return nil, err
	}

	balance, _ := ReadAccountBalance(snapshot, tx.Creator)
	stake, _ := ReadAccountStake(snapshot, tx.Creator)
	reward, _ := ReadAccountReward(snapshot, tx.Creator)

	switch params.Opcode {
	case sys.PlaceStake:
		if balance < params.Amount {
			return nil, errors.Errorf("stake: %x attempt to place a stake of %d PERLs, but only has %d PERLs", tx.Creator, params.Amount, balance)
		}

		WriteAccountBalance(snapshot, tx.Creator, balance-params.Amount)
		WriteAccountStake(snapshot, tx.Creator, stake+params.Amount)
	case sys.WithdrawStake:
		if stake < params.Amount {
			return nil, errors.Errorf("stake: %x attempt to withdraw a stake of %d PERLs, but only has staked %d PERLs", tx.Creator, params.Amount, stake)
		}

		WriteAccountBalance(snapshot, tx.Creator, balance+params.Amount)
		WriteAccountStake(snapshot, tx.Creator, stake-params.Amount)
	case sys.WithdrawReward:
		if params.Amount < sys.MinimumRewardWithdraw {
			return nil, errors.Errorf("stake: %x attempt to withdraw rewards amounting to %d PERLs, but system requires the minimum amount to withdraw to be %d PERLs", tx.Creator, params.Amount, sys.MinimumRewardWithdraw)
		}

		if reward < params.Amount {
			return nil, errors.Errorf("stake: %x attempt to withdraw rewards amounting to %d PERLs, but only has rewards amounting to %d PERLs", tx.Creator, params.Amount, reward)
		}

		WriteAccountReward(snapshot, tx.Creator, reward-params.Amount)
		StoreRewardWithdrawalRequest(snapshot, RewardWithdrawalRequest{
			account: tx.Creator,
			amount:  params.Amount,
			round:   round.Index,
		})
	}

	return snapshot, nil
}

func ApplyContractTransaction(snapshot *avl.Tree, round *Round, tx *Transaction, state *ContractExecutorState) (*avl.Tree, error) {
	params, err := ParseContractTransaction(tx.Payload)
	if err != nil {
		return nil, err
	}

	if _, exists := ReadAccountContractNumPages(snapshot, tx.ID); exists {
		return nil, errors.New("contract: already exists")
	}

	sender := tx.Creator
	if state != nil {
		sender = state.Sender
		params.GasLimit = state.GasLimit
	}

	if params.GasLimit == 0 {
		return nil, errors.New("contract: gas limit for invoking smart contract must be greater than zero")
	}

	balance, _ := ReadAccountBalance(snapshot, sender)

	if balance < params.GasLimit {
		return nil, errors.Errorf("contract: %x tried to spawn a contract using a gas limit of %d PERLs but only has %d PERLs", params.GasLimit, balance)
	}

	executor := &ContractExecutor{}

	if err := executor.Execute(snapshot, tx.ID, round, tx, 0, params.GasLimit, `init`, params.Params, params.Code); err != nil {
		return nil, errors.Wrap(err, "contract: failed to init smart contract")
	}

	WriteAccountBalance(snapshot, tx.Creator, balance-executor.Gas)

	if !executor.GasLimitExceeded {
		if state == nil {
			state = &ContractExecutorState{Sender: tx.Sender}
		}

		if params.GasLimit > executor.Gas {
			state.GasLimit = params.GasLimit - executor.Gas
		}

		for _, entry := range executor.Queue {
			switch entry.Tag {
			case sys.TagNop:
			case sys.TagTransfer:
				if _, err := ApplyTransferTransaction(snapshot, round, entry, state); err != nil {
					return nil, err
				}
			case sys.TagStake:
				if _, err := ApplyStakeTransaction(snapshot, round, entry); err != nil {
					return nil, err
				}
			case sys.TagContract:
				if _, err := ApplyContractTransaction(snapshot, round, entry, state); err != nil {
					return nil, err
				}
			case sys.TagBatch:
				if _, err := ApplyBatchTransaction(snapshot, round, entry); err != nil {
					return nil, err
				}
			}
		}

		WriteAccountContractCode(snapshot, tx.ID, params.Code)
	}

	logger := log.Contracts("gas")
	logger.Info().
		Hex("creator_id", tx.Creator[:]).
		Hex("contract_id", tx.ID[:]).
		Uint64("gas", executor.Gas).
		Uint64("gas_limit", params.GasLimit).
		Msg("Deducted PERLs for spawning a smart contract.")

	return snapshot, nil
}

func ApplyBatchTransaction(snapshot *avl.Tree, round *Round, tx *Transaction) (*avl.Tree, error) {
	params, err := ParseBatchTransaction(tx.Payload)
	if err != nil {
		return nil, err
	}

	for i := uint8(0); i < params.Size; i++ {
		entry := &Transaction{
			ID:      tx.ID,
			Sender:  tx.Sender,
			Creator: tx.Creator,
			Nonce:   tx.Nonce,
			Tag:     params.Tags[i],
			Payload: params.Payloads[i],
		}

		switch entry.Tag {
		case sys.TagNop:
		case sys.TagTransfer:
			if _, err := ApplyTransferTransaction(snapshot, round, entry, nil); err != nil {
				return nil, err
			}
		case sys.TagStake:
			if _, err := ApplyStakeTransaction(snapshot, round, entry); err != nil {
				fmt.Println(err)
				return nil, err
			}
		case sys.TagContract:
			if _, err := ApplyContractTransaction(snapshot, round, entry, nil); err != nil {
				return nil, err
			}
		}
	}

	return snapshot, nil
}
