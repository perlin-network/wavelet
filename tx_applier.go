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
	wasm "github.com/perlin-network/life/wasm-validation"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

type contractExecutorState struct {
	GasPayer      AccountID
	GasLimit      uint64
	GasLimitIsSet bool
	Context       *CollapseContext
}

// Apply the transaction and immediately write the states into the tree.
// If you have many transactions to apply, consider using CollapseContext.
func ApplyTransaction(tree *avl.Tree, block *Block, tx *Transaction) error {
	ctx := NewCollapseContext(tree)
	if err := ctx.ApplyTransaction(block, tx); err != nil {
		return err
	}

	return ctx.Flush()
}

func applyTransaction(block *Block, ctx *CollapseContext, tx *Transaction, executorState *contractExecutorState) error {
	switch tx.Tag {
	case sys.TagTransfer:
		if err := applyTransferTransaction(ctx, block, tx, executorState); err != nil {
			return errors.Wrap(err, "could not apply transfer transaction")
		}
	case sys.TagStake:
		if err := applyStakeTransaction(ctx, block, tx); err != nil {
			return errors.Wrap(err, "could not apply stake transaction")
		}
	case sys.TagContract:
		if err := applyContractTransaction(ctx, block, tx, executorState); err != nil {
			return errors.Wrap(err, "could not apply contract transaction")
		}
	case sys.TagBatch:
		if err := applyBatchTransaction(ctx, block, tx, executorState); err != nil {
			return errors.Wrap(err, "could not apply batch transaction")
		}
	}

	return nil
}

func applyTransferTransaction(ctx *CollapseContext, block *Block, tx *Transaction, state *contractExecutorState) error {
	payload, err := ParseTransfer(tx.Payload)
	if err != nil {
		return err
	}

	code, codeAvailable := ctx.ReadAccountContractCode(payload.Recipient)

	if !codeAvailable && (payload.GasLimit > 0 || len(payload.FuncName) > 0 || len(payload.FuncParams) > 0) {
		return errors.New("transfer: transactions to non-contract accounts should not specify gas limit or function names or params")
	}

	// FIXME(kenta): FOR TESTNET ONLY. FAUCET DOES NOT GET ANY PERLs DEDUCTED.
	if hex.EncodeToString(tx.Sender[:]) == sys.FaucetAddress {
		recipientBalance, _ := ctx.ReadAccountBalance(payload.Recipient)
		ctx.WriteAccountBalance(payload.Recipient, recipientBalance+payload.Amount)

		return nil
	}

	err = transferValue(
		"PERL",
		tx.Sender, payload.Recipient,
		payload.Amount,
		ctx.ReadAccountBalance, ctx.WriteAccountBalance,
		ctx.ReadAccountBalance, ctx.WriteAccountBalance,
	)
	if err != nil {
		return errors.Wrap(err, "failed to execute transferValue on balance")
	}

	if !codeAvailable {
		return nil
	}

	if payload.GasDeposit != 0 {
		err = transferValue(
			"PERL (Gas Deposit)",
			tx.Sender, payload.Recipient,
			payload.GasDeposit,
			ctx.ReadAccountBalance, ctx.WriteAccountBalance,
			ctx.ReadAccountContractGasBalance, ctx.WriteAccountContractGasBalance,
		)
		if err != nil {
			return errors.Wrap(err, "failed to execute transferValue on gas deposit")
		}
	}

	if len(payload.FuncName) == 0 {
		return nil
	}

	return executeContractInTransactionContext(tx, payload.Recipient, code, ctx, block, payload.Amount, payload.GasLimit, payload.FuncName, payload.FuncParams, state)
}

func applyStakeTransaction(ctx *CollapseContext, block *Block, tx *Transaction) error {
	payload, err := ParseStake(tx.Payload)
	if err != nil {
		return err
	}

	balance, _ := ctx.ReadAccountBalance(tx.Sender)
	stake, _ := ctx.ReadAccountStake(tx.Sender)
	reward, _ := ctx.ReadAccountReward(tx.Sender)

	switch payload.Opcode {
	case sys.PlaceStake:
		if balance < payload.Amount {
			return errors.Errorf("stake: %x attempt to place a stake of %d PERLs, but only has %d PERLs", tx.Sender, payload.Amount, balance)
		}

		ctx.WriteAccountBalance(tx.Sender, balance-payload.Amount)
		ctx.WriteAccountStake(tx.Sender, stake+payload.Amount)
	case sys.WithdrawStake:
		if stake < payload.Amount {
			return errors.Errorf("stake: %x attempt to withdraw a stake of %d PERLs, but only has staked %d PERLs", tx.Sender, payload.Amount, payload)
		}

		ctx.WriteAccountBalance(tx.Sender, balance+payload.Amount)
		ctx.WriteAccountStake(tx.Sender, stake-payload.Amount)
	case sys.WithdrawReward:
		if payload.Amount < sys.MinimumRewardWithdraw {
			return errors.Errorf("stake: %x attempt to withdraw rewards amounting to %d PERLs, but system requires the minimum amount to withdraw to be %d PERLs", tx.Sender, payload.Amount, sys.MinimumRewardWithdraw)
		}

		if reward < payload.Amount {
			return errors.Errorf("stake: %x attempt to withdraw rewards amounting to %d PERLs, but only has rewards amounting to %d PERLs", tx.Sender, payload.Amount, reward)
		}

		ctx.WriteAccountReward(tx.Sender, reward-payload.Amount)
		ctx.StoreRewardWithdrawalRequest(RewardWithdrawalRequest{
			account:    tx.Sender,
			amount:     payload.Amount,
			blockIndex: block.Index,
		})
	}

	return nil
}

func applyContractTransaction(ctx *CollapseContext, block *Block, tx *Transaction, state *contractExecutorState) error {
	payload, err := ParseContract(tx.Payload)
	if err != nil {
		return err
	}

	if _, exists := ctx.ReadAccountContractCode(tx.ID); exists {
		return errors.New("contract: already exists")
	}

	// Record the code of the smart contract into the ledgers state.
	if err := wasm.GetValidator().ValidateWasm(payload.Code); err != nil {
		return errors.Wrap(err, "invalid wasm")
	}

	ctx.WriteAccountContractCode(tx.ID, payload.Code)

	if payload.GasDeposit != 0 {
		err = transferValue(
			"PERL (Gas Deposit)",
			tx.Sender, tx.ID,
			payload.GasDeposit,
			ctx.ReadAccountBalance, ctx.WriteAccountBalance,
			ctx.ReadAccountContractGasBalance, ctx.WriteAccountContractGasBalance,
		)
		if err != nil {
			return errors.Wrap(err, "failed to execute transferValue on gas deposit")
		}
	}

	return executeContractInTransactionContext(tx, tx.ID, payload.Code, ctx, block, 0, payload.GasLimit, []byte("init"), payload.Params, state)
}

func applyBatchTransaction(ctx *CollapseContext, block *Block, tx *Transaction, state *contractExecutorState) error {
	payload, err := ParseBatch(tx.Payload)
	if err != nil {
		return err
	}

	for i := uint8(0); i < payload.Size; i++ {
		entry := &Transaction{
			ID:      tx.ID,
			Sender:  tx.Sender,
			Nonce:   tx.Nonce,
			Tag:     sys.Tag(payload.Tags[i]),
			Payload: payload.Payloads[i],
		}
		if err := applyTransaction(block, ctx, entry, state); err != nil {
			return errors.Wrapf(err, "Error while processing %d/%d transaction in a batch.", i+1, payload.Size)
		}
	}

	return nil
}

// Transfers value of any form (balance, gasDeposit/gasBalance).
func transferValue(
	unitName string,
	from, to AccountID,
	amount uint64,
	srcRead func(AccountID) (uint64, bool), srcWrite func(AccountID, uint64),
	dstRead func(AccountID) (uint64, bool), dstWrite func(AccountID, uint64),
) error {
	senderValue, _ := srcRead(from)

	if senderValue < amount {
		return errors.Errorf("transfer_value: %x tried send %d %s to %x, but only has %d %s",
			from, amount, unitName, to, senderValue, unitName)
	}

	senderValue -= amount
	srcWrite(from, senderValue)

	recipientValue, _ := dstRead(to)
	recipientValue += amount
	dstWrite(to, recipientValue)

	return nil
}

func executeContractInTransactionContext(
	tx *Transaction,
	contractID AccountID,
	code []byte,
	ctx *CollapseContext,
	block *Block,
	amount uint64,
	requestedGasLimit uint64,
	funcName []byte,
	funcParams []byte,
	state *contractExecutorState,
) error {
	logger := log.Contracts("execute")

	gasPayerBalance, _ := ctx.ReadAccountBalance(state.GasPayer)
	contractGasBalance, _ := ctx.ReadAccountContractGasBalance(contractID)
	availableBalance := gasPayerBalance + contractGasBalance

	if !state.GasLimitIsSet {
		state.GasLimit = requestedGasLimit
		state.GasLimitIsSet = true
	}

	realGasLimit := state.GasLimit

	if requestedGasLimit < realGasLimit {
		realGasLimit = requestedGasLimit
	}

	if realGasLimit == 0 {
		return errors.New("execute_contract: gas limit for invoking smart contract function must be greater than zero")
	}

	if availableBalance < realGasLimit {
		return errors.Errorf("execute_contract: attempted to deduct gas fee from %x of %d PERLs, but only has %d PERLs",
			state.GasPayer, realGasLimit, availableBalance)
	}

	executor := &ContractExecutor{}

	var contractState *VMState
	contractState, _ = ctx.GetContractState(contractID)

	newContractState, invocationErr := executor.Execute(contractID, block, tx, amount, realGasLimit, string(funcName), funcParams, code, ctx.tree, ctx.VMCache, contractState)

	// availableBalance >= realGasLimit >= executor.Gas && state.GasLimit >= realGasLimit must always hold.
	if realGasLimit < executor.Gas {
		logger.Fatal().Msg("BUG: realGasLimit < executor.Gas")
	}

	if state.GasLimit < realGasLimit {
		logger.Fatal().Msg("BUG: state.GasLimit < realGasLimit")
	}

	if invocationErr != nil { // Revert changes and have the gas payer pay gas fees.
		if executor.Gas > contractGasBalance {
			ctx.WriteAccountContractGasBalance(contractID, 0)

			if gasPayerBalance < (executor.Gas - contractGasBalance) {
				logger.Fatal().Msg("BUG: gasPayerBalance < (executor.Gas - contractGasBalance)")
			}

			ctx.WriteAccountBalance(state.GasPayer, gasPayerBalance-(executor.Gas-contractGasBalance))
		} else {
			ctx.WriteAccountContractGasBalance(contractID, contractGasBalance-executor.Gas)
		}

		state.GasLimit -= executor.Gas

		if executor.GasLimitExceeded {
			logger.Info().
				Hex("sender_id", tx.Sender[:]).
				Hex("contract_id", contractID[:]).
				Uint64("gas", executor.Gas).
				Uint64("gas_limit", realGasLimit).
				Msg("Exceeded gas limit while invoking smart contract function.")
		} else {
			logger.Info().Err(invocationErr).Msg("failed to invoke smart contract")
		}
	} else {
		// Contract invocation succeeded. VM state can be safely saved now.
		ctx.SetContractState(contractID, newContractState)

		if executor.Gas > contractGasBalance {
			ctx.WriteAccountContractGasBalance(contractID, 0)
			if gasPayerBalance < (executor.Gas - contractGasBalance) {
				logger.Fatal().Msg("BUG: gasPayerBalance < (executor.Gas - contractGasBalance)")
			}
			ctx.WriteAccountBalance(state.GasPayer, gasPayerBalance-(executor.Gas-contractGasBalance))
		} else {
			ctx.WriteAccountContractGasBalance(contractID, contractGasBalance-executor.Gas)
		}
		state.GasLimit -= executor.Gas

		//logger.Info().
		//	Uint64("gas", executor.Gas).
		//	Uint64("gas_limit", realGasLimit).
		//	Msg("Deducted PERLs for invoking smart contract function.")

		for _, entry := range executor.Queue {
			err := applyTransaction(block, ctx, entry, state)
			if err != nil {
				logger.Info().Err(err).Msg("failed to process sub-transaction")
			}
		}
	}

	return nil
}
