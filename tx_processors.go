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
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
)

func ProcessNopTransaction(_ *TransactionContext) error {
	return nil
}

func ProcessTransferTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

	r := bytes.NewReader(tx.Payload)

	var recipient AccountID

	if _, err := io.ReadFull(r, recipient[:]); err != nil {
		return errors.Wrap(err, "transfer: failed to decode recipient")
	}

	var buf [8]byte

	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return errors.Wrap(err, "transfer: failed to decode amount to transfer")
	}

	amount := binary.LittleEndian.Uint64(buf[:])

	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return errors.Wrap(err, "transfer: failed to decode gas limit of transfer")
	}

	gasLimit := binary.LittleEndian.Uint64(buf[:])

	creatorBalance, _ := ctx.ReadAccountBalance(tx.Creator)

	if creatorBalance < amount+gasLimit {
		return errors.Errorf(
			"transfer: transaction creator tried to send %d PERLs with %d gas limit, but only has %d PERLs",
			amount, gasLimit, creatorBalance,
		)
	}

	ctx.WriteAccountBalance(tx.Creator, creatorBalance-amount)

	recipientBalance, _ := ctx.ReadAccountBalance(recipient)
	ctx.WriteAccountBalance(recipient, recipientBalance+amount)

	if _, isContract := ctx.ReadAccountContractCode(recipient); !isContract {
		return nil
	}

	executor := NewContractExecutor(recipient, ctx).WithGasTable(sys.GasTable).EnableLogging()

	var (
		gas uint64
		err error
	)

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, buf[:4]); err != nil {
			return errors.Wrap(err, "transfer: failed to decode size of smart contract function name to invoke")
		}

		funcName := make([]byte, binary.LittleEndian.Uint32(buf[:4]))

		if _, err := io.ReadFull(r, funcName); err != nil {
			return errors.Wrap(err, "transfer: failed to decode smart contract function name to invoke")
		}

		var funcParams []byte
		if r.Len() > 0 {
			if _, err := io.ReadFull(r, buf[:4]); err != nil {
				return errors.Wrap(err, "transfer: failed to decode number of smart contract function invocation parameters")
			}

			funcParams = make([]byte, binary.LittleEndian.Uint32(buf[:4]))

			if _, err := io.ReadFull(r, funcParams); err != nil {
				return errors.Wrap(err, "transfer: failed to decode smart contract function invocation parameters")
			}
		}

		gas, err = executor.Run(amount, gasLimit, string(funcName), funcParams...)
	} else {
		gas, err = executor.Run(amount, gasLimit, "on_money_received")
	}

	if err != nil && errors.Cause(err) != ErrContractFunctionNotFound {
		return errors.Wrap(err, "transfer: failed to execute smart contract method")
	}

	ctx.WriteAccountBalance(tx.Creator, creatorBalance-amount-gas)

	logger := log.Contracts("gas")
	logger.Info().
		Hex("contract_id", recipient[:]).
		Uint64("gas", gas).
		Hex("creator_id", tx.Creator[:]).
		Hex("contract_id", recipient[:]).
		Msg("Deducted PERLs for invoking smart contract function.")

	return nil
}

func ProcessStakeTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

	r := bytes.NewReader(tx.Payload)

	var buf [9]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return errors.Wrap(err, "stake: failed to decode amount of stake to place/withdraw")
	}

	delta := binary.LittleEndian.Uint64(buf[1:])

	balance, _ := ctx.ReadAccountBalance(tx.Creator)
	stake, _ := ctx.ReadAccountStake(tx.Creator)

	switch buf[0] {
	case sys.PlaceStake:
		if balance < delta {
			return errors.New("stake: balance < delta")
		}

		ctx.WriteAccountBalance(tx.Creator, balance-delta)
		ctx.WriteAccountStake(tx.Creator, stake+delta)
	case sys.WithdrawStake:
		if stake < delta {
			return errors.New("stake: stake < delta")
		}

		ctx.WriteAccountBalance(tx.Creator, balance+delta)
		ctx.WriteAccountStake(tx.Creator, stake-delta)
	case sys.WithdrawReward:
		if delta < sys.MinimumRewardWithdraw {
			return fmt.Errorf("%d is less than minimum amount of reward to withdraw (%d)", delta, sys.MinimumRewardWithdraw)
		}

		reward, _ := ctx.ReadAccountReward(tx.Creator)
		if reward < delta {
			return errors.New("reward: reward < delta")
		}

		ctx.WriteRewardWithdrawRequest(tx.Creator, delta)
	default:
		return fmt.Errorf("unrecognised stake transaction type - %x", buf[0])
	}

	return nil
}

func ProcessContractTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

	if _, exists := ctx.ReadAccountContractCode(tx.ID); exists {
		return errors.New("contract: there already exists a contract spawned with the specified code")
	}

	r := bytes.NewReader(tx.Payload)

	var buf [8]byte

	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return errors.Wrap(err, "contract: failed to read gas limit from payload")
	}

	gasLimit := binary.LittleEndian.Uint64(buf[:])

	if gasLimit == 0 {
		return errors.New("contract: gas limit greater than zero must be specified for spawning a smart contract")
	}

	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return errors.Wrap(err, "contract: failed to read input parameters length")
	}

	payload := make([]byte, binary.LittleEndian.Uint64(buf[:]))

	if _, err := io.ReadFull(r, payload[:]); err != nil {
		return errors.Wrap(err, "contract: failed to read input parameters")
	}

	code, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "contract: failed to read contract code")
	}

	balance, _ := ctx.ReadAccountBalance(tx.Creator)

	storagePrice := uint64(len(tx.Payload)) / sys.GasTable["wavelet.contract.spawn.cost"]

	if storagePrice < sys.GasTable["wavelet.contract.spawn.min"] {
		storagePrice = sys.GasTable["wavelet.contract.spawn.min"]
	}

	if balance < storagePrice+gasLimit {
		return errors.Errorf("contract: transaction creator must have %d PERLs to spawn contract, but they only have %d PERLs", storagePrice, balance)
	}

	gas, err := NewContractExecutor(tx.ID, ctx).WithGasTable(sys.GasTable).EnableLogging().Spawn(code, payload, gasLimit)

	ctx.WriteAccountBalance(tx.Creator, balance-storagePrice-gas)

	if err != nil {
		return errors.Wrap(err, "contract: failed to init smart contract")
	}

	ctx.WriteAccountContractCode(tx.ID, code)

	return nil
}

func ProcessBatchTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

	if len(tx.Payload) == 0 {
		return errors.New("batch: payload must not be empty for batch transaction")
	}

	reader := bytes.NewReader(tx.Payload)

	var buf [4]byte

	if _, err := io.ReadFull(reader, buf[:1]); err != nil {
		return errors.Wrap(err, "batch: length not specified in batch transaction")
	}

	size := int(buf[0])

	for i := 0; i < size; i++ {
		if _, err := io.ReadFull(reader, buf[:1]); err != nil {
			return errors.Wrap(err, "batch: could not read tag")
		}

		tag := buf[0]

		if _, err := io.ReadFull(reader, buf[:4]); err != nil {
			return errors.Wrap(err, "batch: could not read payload size")
		}

		size := binary.BigEndian.Uint32(buf[:4])

		payload := make([]byte, size)

		if _, err := io.ReadFull(reader, payload[:]); err != nil {
			return errors.Wrap(err, "batch: could not read payload")
		}

		var item Transaction

		item.ID = tx.ID
		item.Sender = tx.Sender
		item.Creator = tx.Creator
		item.Nonce = tx.Nonce
		item.Tag = tag
		item.Payload = payload

		ctx.SendTransaction(&item)
	}

	return nil
}
