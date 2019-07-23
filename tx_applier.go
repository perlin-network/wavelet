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
	wasm_validation "github.com/perlin-network/life/wasm-validation"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

type contractExecutorState struct {
	GasPayer      AccountID
	GasLimit      uint64
	GasLimitIsSet bool
}

func ApplyTransaction(round *Round, state *avl.Tree, tx *Transaction) error {
	return applyTransaction(round, state, tx, &contractExecutorState{
		GasPayer: tx.Creator,
	})
}

func applyTransaction(round *Round, state *avl.Tree, tx *Transaction, execState *contractExecutorState) error {
	original := state.Snapshot()

	switch tx.Tag {
	case sys.TagNop:
	case sys.TagTransfer:
		if err := applyTransferTransaction(state, round, tx, execState); err != nil {
			state.Revert(original)
			return errors.Wrap(err, "could not apply transfer transaction")
		}
	case sys.TagStake:
		if err := applyStakeTransaction(state, round, tx); err != nil {
			state.Revert(original)
			return errors.Wrap(err, "could not apply stake transaction")
		}
	case sys.TagContract:
		if err := applyContractTransaction(state, round, tx, execState); err != nil {
			state.Revert(original)
			return errors.Wrap(err, "could not apply contract transaction")
		}
	case sys.TagBatch:
		if err := applyBatchTransaction(state, round, tx, execState); err != nil {
			state.Revert(original)
			return errors.Wrap(err, "could not apply batch transaction")
		}
	}

	return nil
}

func applyTransferTransaction(snapshot *avl.Tree, round *Round, tx *Transaction, state *contractExecutorState) error {
	params, err := ParseTransferTransaction(tx.Payload)
	if err != nil {
		return err
	}

	code, codeAvailable := ReadAccountContractCode(snapshot, params.Recipient)

	if !codeAvailable && (params.GasLimit > 0 || len(params.FuncName) > 0 || len(params.FuncParams) > 0) {
		return errors.New("transfer: transactions to non-contract accounts should not specify gas limit or function names or params")
	}

	// senderBalance/recipientBalance should be hidden from other code.
	{
		senderBalance, _ := ReadAccountBalance(snapshot, tx.Creator)

		// FIXME(kenta): FOR TESTNET ONLY. FAUCET DOES NOT GET ANY PERLs DEDUCTED.
		if hex.EncodeToString(tx.Creator[:]) == sys.FaucetAddress {
			recipientBalance, _ := ReadAccountBalance(snapshot, params.Recipient)
			WriteAccountBalance(snapshot, params.Recipient, recipientBalance+params.Amount)

			return nil
		}

		if senderBalance < params.Amount {
			return errors.Errorf("transfer: %x tried send %d PERLs to %x, but only has %d PERLs",
				tx.Creator, params.Amount, params.Recipient, senderBalance)
		}

		senderBalance -= params.Amount
		WriteAccountBalance(snapshot, tx.Creator, senderBalance)

		recipientBalance, _ := ReadAccountBalance(snapshot, params.Recipient)
		recipientBalance += params.Amount
		WriteAccountBalance(snapshot, params.Recipient, recipientBalance)
	}

	if !codeAvailable {
		return nil
	}

	err = executeContractInTransactionContext(tx, params.Recipient, code, snapshot, round, params.Amount, params.GasLimit, params.FuncName, params.FuncParams, state)
	return err
}

func applyStakeTransaction(snapshot *avl.Tree, round *Round, tx *Transaction) error {
	params, err := ParseStakeTransaction(tx.Payload)
	if err != nil {
		return err
	}

	balance, _ := ReadAccountBalance(snapshot, tx.Creator)
	stake, _ := ReadAccountStake(snapshot, tx.Creator)
	reward, _ := ReadAccountReward(snapshot, tx.Creator)

	switch params.Opcode {
	case sys.PlaceStake:
		if balance < params.Amount {
			return errors.Errorf("stake: %x attempt to place a stake of %d PERLs, but only has %d PERLs", tx.Creator, params.Amount, balance)
		}

		WriteAccountBalance(snapshot, tx.Creator, balance-params.Amount)
		WriteAccountStake(snapshot, tx.Creator, stake+params.Amount)
	case sys.WithdrawStake:
		if stake < params.Amount {
			return errors.Errorf("stake: %x attempt to withdraw a stake of %d PERLs, but only has staked %d PERLs", tx.Creator, params.Amount, stake)
		}

		WriteAccountBalance(snapshot, tx.Creator, balance+params.Amount)
		WriteAccountStake(snapshot, tx.Creator, stake-params.Amount)
	case sys.WithdrawReward:
		if params.Amount < sys.MinimumRewardWithdraw {
			return errors.Errorf("stake: %x attempt to withdraw rewards amounting to %d PERLs, but system requires the minimum amount to withdraw to be %d PERLs", tx.Creator, params.Amount, sys.MinimumRewardWithdraw)
		}

		if reward < params.Amount {
			return errors.Errorf("stake: %x attempt to withdraw rewards amounting to %d PERLs, but only has rewards amounting to %d PERLs", tx.Creator, params.Amount, reward)
		}

		WriteAccountReward(snapshot, tx.Creator, reward-params.Amount)
		StoreRewardWithdrawalRequest(snapshot, RewardWithdrawalRequest{
			account: tx.Creator,
			amount:  params.Amount,
			round:   round.Index,
		})
	}

	return nil
}

func applyContractTransaction(snapshot *avl.Tree, round *Round, tx *Transaction, state *contractExecutorState) error {
	params, err := ParseContractTransaction(tx.Payload)
	if err != nil {
		return err
	}

	if _, exists := ReadAccountContractNumPages(snapshot, tx.ID); exists {
		return errors.New("contract: already exists")
	}

	if err := wasm_validation.GetValidator().ValidateWasm(params.Code); err != nil {
		return errors.Wrap(err, "invalid wasm")
	}

	WriteAccountContractCode(snapshot, tx.ID, params.Code)
	err = executeContractInTransactionContext(tx, AccountID(tx.ID), params.Code, snapshot, round, 0, params.GasLimit, []byte("init"), params.Params, state)
	return err
}

func applyBatchTransaction(snapshot *avl.Tree, round *Round, tx *Transaction, state *contractExecutorState) error {
	params, err := ParseBatchTransaction(tx.Payload)
	if err != nil {
		return err
	}

	for i := uint8(0); i < params.Size; i++ {
		entry := &Transaction{
			ID:      tx.ID,
			Sender:  tx.Sender,
			Creator: tx.Creator,
			Nonce:   tx.Nonce,
			Tag:     sys.Tag(params.Tags[i]),
			Payload: params.Payloads[i],
		}
		if err := applyTransaction(round, snapshot, entry, state); err != nil {
			return errors.Wrapf(err, "Error while processing %d/%d transaction in a batch.", i+1, params.Size)
		}
	}

	return nil
}

func executeContractInTransactionContext(
	tx *Transaction,
	contractID AccountID,
	code []byte,
	snapshot *avl.Tree,
	round *Round,
	amount uint64,
	requestedGasLimit uint64,
	funcName []byte,
	funcParams []byte,
	state *contractExecutorState,
) error {
	logger := log.Contracts("execute")

	gasPayer := state.GasPayer
	gasPayerBalance, _ := ReadAccountBalance(snapshot, gasPayer)

	if !state.GasLimitIsSet {
		state.GasLimit = requestedGasLimit
		state.GasLimitIsSet = true
	}

	realGasLimit := state.GasLimit

	if requestedGasLimit < realGasLimit {
		realGasLimit = requestedGasLimit
	}

	if realGasLimit == 0 {
		return errors.New("execute_contract: gas limit for invoking smart contract must be greater than zero")
	}

	if gasPayerBalance < realGasLimit {
		return errors.Errorf("execute_contract: attempted to deduct gas fee from %x of %d PERLs, but only has %d PERLs",
			gasPayer, realGasLimit, gasPayerBalance)
	}

	executor := &ContractExecutor{}
	snapshotBeforeExec := snapshot.Snapshot()

	invocationErr := executor.Execute(snapshot, contractID, round, tx, amount, realGasLimit, string(funcName), funcParams, code)

	// gasPayerBalance >= realGasLimit >= executor.Gas && state.GasLimit >= realGasLimit must always hold.
	if realGasLimit < executor.Gas {
		logger.Fatal().Msg("BUG: realGasLimit < executor.Gas")
	}
	if state.GasLimit < realGasLimit {
		logger.Fatal().Msg("BUG: state.GasLimit < realGasLimit")
	}

	if executor.GasLimitExceeded || invocationErr != nil { // Revert changes and have the gas payer pay gas fees.
		snapshot.Revert(snapshotBeforeExec)
		WriteAccountBalance(snapshot, gasPayer, gasPayerBalance-executor.Gas)
		state.GasLimit -= executor.Gas

		if invocationErr != nil {
			logger.Info().Err(invocationErr).Msg("failed to invoke smart contract")
		} else {
			logger.Info().
				Hex("sender_id", tx.Creator[:]).
				Hex("contract_id", contractID[:]).
				Uint64("gas", executor.Gas).
				Uint64("gas_limit", realGasLimit).
				Msg("Exceeded gas limit while invoking smart contract function.")
		}
	} else {
		WriteAccountBalance(snapshot, gasPayer, gasPayerBalance-executor.Gas)
		state.GasLimit -= executor.Gas

		logger.Info().
			Hex("sender_id", tx.Creator[:]).
			Hex("contract_id", contractID[:]).
			Uint64("gas", executor.Gas).
			Uint64("gas_limit", realGasLimit).
			Msg("Deducted PERLs for invoking smart contract function.")

		for _, entry := range executor.Queue {
			err := applyTransaction(round, snapshot, entry, state)
			if err != nil {
				logger.Info().Err(err).Msg("failed to process sub-transaction")
			}
		}
	}

	return nil
}
