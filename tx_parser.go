package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
)

type Transfer struct {
	Recipient AccountID
	Amount    uint64
	GasLimit  uint64

	FuncName   []byte
	FuncParams []byte
}

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
	}

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, b[:4]); err != nil {
			return tx, errors.Wrap(err, "transfer: failed to decode size of smart contract function name to invoke")
		}

		tx.FuncName = make([]byte, binary.LittleEndian.Uint32(b[:4]))

		if _, err := io.ReadFull(r, tx.FuncName); err != nil {
			return tx, errors.Wrap(err, "transfer: failed to decode smart contract function name to invoke")
		}
	}

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, b[:4]); err != nil {
			return tx, errors.Wrap(err, "transfer: failed to decode number of smart contract function invocation parameters")
		}

		tx.FuncParams = make([]byte, binary.LittleEndian.Uint32(b[:4]))

		if _, err := io.ReadFull(r, tx.FuncParams); err != nil {
			return tx, errors.Wrap(err, "transfer: failed to decode smart contract function invocation parameters")
		}
	}

	return tx, nil
}

type Stake struct {
	Opcode byte
	Amount uint64
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

	if tx.Opcode == sys.WithdrawReward && tx.Amount < sys.MinimumRewardWithdraw {
		return tx, errors.Errorf("stake: must withdraw a reward of a minimum of %d PERLs, but requested to withdraw %d PERLs", sys.MinimumRewardWithdraw, tx.Amount)
	}

	return tx, nil
}

type Contract struct {
	GasLimit uint64

	Params []byte
	Code   []byte
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

	if _, err := io.ReadFull(r, b[:4]); err != nil {
		return tx, errors.Wrap(err, "contract: failed to decode number of smart contract init parameters")
	}

	tx.Params = make([]byte, binary.LittleEndian.Uint32(b[:4]))

	if _, err := io.ReadFull(r, tx.Params); err != nil {
		return tx, errors.Wrap(err, "contract: failed to decode smart contract init parameters")
	}

	var err error

	if tx.Code, err = ioutil.ReadAll(r); err != nil {
		return tx, errors.Wrap(err, "contract: failed to decode smart contract code")
	}

	return tx, nil
}

type Batch struct {
	Size     uint8
	Tags     []uint8
	Payloads [][]byte
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
	tx.Tags = make([]uint8, tx.Size)
	tx.Payloads = make([][]byte, tx.Size)

	for i := uint8(0); i < tx.Size; i++ {
		if _, err := io.ReadFull(r, b[:1]); err != nil {
			return tx, errors.Wrap(err, "batch: could not read tag")
		}

		if b[0] == sys.TagBatch {
			return tx, errors.New("batch: entries inside batch cannot be batch transactions themselves")
		}

		tx.Tags[i] = b[0]

		if _, err := io.ReadFull(r, b[:4]); err != nil {
			return tx, errors.Wrap(err, "batch: could not read payload size")
		}

		tx.Payloads[i] = make([]byte, binary.BigEndian.Uint32(b[:4]))

		if _, err := io.ReadFull(r, tx.Payloads[i]); err != nil {
			return tx, errors.Wrap(err, "batch: could not read payload")
		}
	}

	return tx, nil
}
