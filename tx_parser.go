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
	"io"
	"io/ioutil"

	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

type (
	Transfer struct {
		Recipient AccountID
		Amount    uint64

		// The rest of the fields below are only populated
		// should the transaction be made with a recipient
		// that is a smart contract address.

		GasLimit   uint64
		GasDeposit uint64

		FuncName   []byte
		FuncParams []byte
	}

	Stake struct {
		Opcode byte
		Amount uint64
	}

	Contract struct {
		GasLimit   uint64
		GasDeposit uint64

		Params []byte
		Code   []byte
	}

	Batch struct {
		Size     uint8
		Tags     []uint8
		Payloads [][]byte
	}
)

// ParseTransferTransaction parses and performs sanity checks on the payload of a transfer transaction.
func ParseTransferTransaction(payload []byte) (Transfer, error) {
	r := bytes.NewReader(payload)
	b := make([]byte, 8)

	tx := Transfer{}

	if _, err := io.ReadFull(r, tx.Recipient[:]); err != nil {
		return tx, errors.Wrap(err, "transfer: failed to decode recipient")
	}

	if _, err := io.ReadFull(r, b); err != nil {
		return tx, errors.Wrap(err, "transfer: failed to decode amount of PERLs to send")
	}

	tx.Amount = binary.LittleEndian.Uint64(b)

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, b); err != nil {
			return tx, errors.Wrap(err, "transfer: failed to decode gas limit")
		}

		tx.GasLimit = binary.LittleEndian.Uint64(b)

		if tx.GasLimit == 0 {
			return tx, errors.New("transfer: gas limit must be greater than zero")
		}

		if _, err := io.ReadFull(r, b[:8]); err != nil {
			return tx, errors.Wrap(err, "transfer: failed to decode gas deposit")
		}

		tx.GasDeposit = binary.LittleEndian.Uint64(b)
	}

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, b[:4]); err != nil {
			return tx, errors.Wrap(err, "transfer: failed to decode size of smart contract function name to invoke")
		}

		size := binary.LittleEndian.Uint32(b[:4])
		if size > 1024 {
			return tx, errors.New("transfer: smart contract function name exceeds 1024 characters")
		}

		tx.FuncName = make([]byte, size)

		if _, err := io.ReadFull(r, tx.FuncName); err != nil {
			return tx, errors.Wrap(err, "transfer: failed to decode smart contract function name to invoke")
		}

		if string(tx.FuncName) == "init" {
			return tx, errors.New("transfer: not allowed to call init function for smart contract")
		}
	}

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, b[:4]); err != nil {
			return tx, errors.Wrap(err, "transfer: failed to decode number of smart contract function invocation parameters")
		}

		size := binary.LittleEndian.Uint32(b[:4])
		if size > 1024*1024 {
			return tx, errors.New("transfer: smart contract payload exceeds 1MB")
		}

		tx.FuncParams = make([]byte, size)

		if _, err := io.ReadFull(r, tx.FuncParams); err != nil {
			return tx, errors.Wrap(err, "transfer: failed to decode smart contract function invocation parameters")
		}
	}

	return tx, nil
}

// ParseStakeTransaction parses and performs sanity checks on the payload of a stake transaction.
func ParseStakeTransaction(payload []byte) (Stake, error) {
	tx := Stake{}

	if len(payload) != 9 {
		return tx, errors.New("stake: payload must be exactly 9 bytes")
	}

	tx.Opcode = payload[0]

	if tx.Opcode > sys.WithdrawReward {
		return tx, errors.New("stake: opcode must be 0, 1, or 2")
	}

	tx.Amount = binary.LittleEndian.Uint64(payload[1:9])

	if tx.Amount == 0 {
		return tx, errors.New("stake: amount must be greater than zero")
	}

	if tx.Opcode == sys.WithdrawReward && tx.Amount < sys.MinimumRewardWithdraw {
		return tx, errors.Errorf("stake: must withdraw a reward of a minimum of %d PERLs, but requested to withdraw %d PERLs", sys.MinimumRewardWithdraw, tx.Amount)
	}

	return tx, nil
}

// ParseContractTransaction parses and performs sanity checks on the payload of a contract transaction.
func ParseContractTransaction(payload []byte) (Contract, error) {
	r := bytes.NewReader(payload)
	b := make([]byte, 8)

	tx := Contract{}

	if _, err := io.ReadFull(r, b[:8]); err != nil {
		return tx, errors.Wrap(err, "contract: failed to decode gas limit")
	}

	tx.GasLimit = binary.LittleEndian.Uint64(b)

	if tx.GasLimit == 0 {
		return tx, errors.New("contract: gas limit for invoking smart contract must be greater than zero")
	}

	if _, err := io.ReadFull(r, b[:8]); err != nil {
		return tx, errors.Wrap(err, "contract: failed to decode gas deposit")
	}

	tx.GasDeposit = binary.LittleEndian.Uint64(b)

	if _, err := io.ReadFull(r, b[:4]); err != nil {
		return tx, errors.Wrap(err, "contract: failed to decode number of smart contract init parameters")
	}

	size := binary.LittleEndian.Uint32(b[:4])
	if size > 1024*1024 {
		return tx, errors.New("contract: smart contract payload exceeds 1MB")
	}
	tx.Params = make([]byte, size)

	if _, err := io.ReadFull(r, tx.Params); err != nil {
		return tx, errors.Wrap(err, "contract: failed to decode smart contract init parameters")
	}

	var err error

	if tx.Code, err = ioutil.ReadAll(r); err != nil {
		return tx, errors.Wrap(err, "contract: failed to decode smart contract code")
	}

	if len(tx.Code) == 0 {
		return tx, errors.New("contract: smart contract must have code of length greater than zero")
	}

	return tx, nil
}

// ParseBatchTransaction parses and performs sanity checks on the payload of a batch transaction.
func ParseBatchTransaction(payload []byte) (Batch, error) {
	r := bytes.NewReader(payload)
	b := make([]byte, 4)

	tx := Batch{}

	if _, err := io.ReadFull(r, b[:1]); err != nil {
		return tx, errors.Wrap(err, "batch: failed to decode number of transactions in batch")
	}

	tx.Size = b[0]

	if tx.Size == 0 {
		return tx, errors.New("batch: size must be greater than zero")
	}

	tx.Tags = make([]uint8, tx.Size)
	tx.Payloads = make([][]byte, tx.Size)

	for i := uint8(0); i < tx.Size; i++ {
		if _, err := io.ReadFull(r, b[:1]); err != nil {
			return tx, errors.Wrap(err, "batch: could not read tag")
		}

		if sys.Tag(b[0]) == sys.TagBatch {
			return tx, errors.New("batch: entries inside batch cannot be batch transactions themselves")
		}

		tx.Tags[i] = b[0]

		if _, err := io.ReadFull(r, b[:4]); err != nil {
			return tx, errors.Wrap(err, "batch: could not read payload size")
		}

		size := binary.BigEndian.Uint32(b[:4])
		if size > 2*1024*1024 {
			return tx, errors.New("batch: payload size exceeds 2MB")
		}

		tx.Payloads[i] = make([]byte, size)

		if _, err := io.ReadFull(r, tx.Payloads[i]); err != nil {
			return tx, errors.Wrap(err, "batch: could not read payload")
		}
	}

	return tx, nil
}
