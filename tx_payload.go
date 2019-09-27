package wavelet

import (
	"bytes"
	"encoding/binary"
	"fmt"
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

// ParseTransfer parses and performs sanity checks on the payload of a transfer transaction.
func ParseTransfer(payload []byte) (Transfer, error) {
	r := bytes.NewReader(payload)
	b := make([]byte, 8)

	var transfer Transfer

	if _, err := io.ReadFull(r, transfer.Recipient[:]); err != nil {
		return transfer, errors.Wrap(err, "transfer: failed to decode recipient")
	}

	if _, err := io.ReadFull(r, b); err != nil {
		return transfer, errors.Wrap(err, "transfer: failed to decode amount of PERLs to send")
	}

	transfer.Amount = binary.LittleEndian.Uint64(b)

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, b); err != nil {
			return transfer, errors.Wrap(err, "transfer: failed to decode gas limit")
		}

		transfer.GasLimit = binary.LittleEndian.Uint64(b)
	}

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, b[:8]); err != nil {
			return transfer, errors.Wrap(err, "transfer: failed to decode gas deposit")
		}

		transfer.GasDeposit = binary.LittleEndian.Uint64(b)
	}

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, b[:4]); err != nil {
			return transfer, errors.Wrap(err, "transfer: failed to decode size of smart contract function name to invoke")
		}

		size := binary.LittleEndian.Uint32(b[:4])
		if size > 1024 {
			return transfer, errors.New("transfer: smart contract function name exceeds 1024 characters")
		}

		transfer.FuncName = make([]byte, size)

		if _, err := io.ReadFull(r, transfer.FuncName); err != nil {
			return transfer, errors.Wrap(err, "transfer: failed to decode smart contract function name to invoke")
		}

		if string(transfer.FuncName) == "init" {
			return transfer, errors.New("transfer: not allowed to call init function for smart contract")
		}
	}

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, b[:4]); err != nil {
			return transfer, errors.Wrap(err, "transfer: failed to decode number of smart contract function invocation parameters")
		}

		size := binary.LittleEndian.Uint32(b[:4])
		if size > 1*1024*1024 {
			return transfer, errors.New("transfer: smart contract payload exceeds 1MB")
		}

		transfer.FuncParams = make([]byte, size)

		if _, err := io.ReadFull(r, transfer.FuncParams); err != nil {
			return transfer, errors.Wrap(err, "transfer: failed to decode smart contract function invocation parameters")
		}
	}

	if transfer.GasLimit == 0 && len(transfer.FuncName) > 0 {
		return transfer, errors.New("transfer: gas limit for invoking smart contract function must be greater than zero")
	}

	return transfer, nil
}

// ParseStake parses and performs sanity checks on the payload of a stake transaction.
func ParseStake(payload []byte) (Stake, error) {
	var stake Stake

	if len(payload) != 9 {
		return stake, errors.New("stake: payload must be exactly 9 bytes")
	}

	stake.Opcode = payload[0]

	if stake.Opcode > sys.WithdrawReward {
		return stake, errors.New("stake: opcode must be 0, 1, or 2")
	}

	stake.Amount = binary.LittleEndian.Uint64(payload[1:9])

	if stake.Amount == 0 {
		return stake, errors.New("stake: amount must be greater than zero")
	}

	if stake.Opcode == sys.WithdrawReward && stake.Amount < sys.MinimumRewardWithdraw {
		return stake, errors.Errorf("stake: must withdraw a reward of a minimum of %d PERLs, but requested to withdraw %d PERLs", sys.MinimumRewardWithdraw, stake.Amount)
	}

	return stake, nil
}

// ParseContract parses and performs sanity checks on the payload of a contract transaction.
func ParseContract(payload []byte) (Contract, error) {
	r := bytes.NewReader(payload)
	b := make([]byte, 8)

	var contract Contract

	if _, err := io.ReadFull(r, b[:8]); err != nil {
		return contract, errors.Wrap(err, "contract: failed to decode gas limit")
	}

	contract.GasLimit = binary.LittleEndian.Uint64(b)

	if _, err := io.ReadFull(r, b[:8]); err != nil {
		return contract, errors.Wrap(err, "contract: failed to decode gas deposit")
	}

	if contract.GasLimit == 0 {
		return contract, errors.New("contract: gas limit for invoking smart contract function must be greater than zero")
	}

	contract.GasDeposit = binary.LittleEndian.Uint64(b)

	if _, err := io.ReadFull(r, b[:4]); err != nil {
		return contract, errors.Wrap(err, "contract: failed to decode number of smart contract init parameters")
	}

	size := binary.LittleEndian.Uint32(b[:4])
	if size > 1024*1024 {
		return contract, errors.New("contract: smart contract payload exceeds 1MB")
	}
	contract.Params = make([]byte, size)

	if _, err := io.ReadFull(r, contract.Params); err != nil {
		return contract, errors.Wrap(err, "contract: failed to decode smart contract init parameters")
	}

	var err error

	if contract.Code, err = ioutil.ReadAll(r); err != nil {
		return contract, errors.Wrap(err, "contract: failed to decode smart contract code")
	}

	if len(contract.Code) == 0 {
		return contract, errors.New("contract: smart contract must have code of length greater than zero")
	}

	return contract, nil
}

// ParseBatch parses and performs sanity checks on the payload of a batch transaction.
func ParseBatch(payload []byte) (Batch, error) {
	r := bytes.NewReader(payload)
	b := make([]byte, 4)

	var batch Batch

	if _, err := io.ReadFull(r, b[:1]); err != nil {
		return batch, errors.Wrap(err, "batch: failed to decode number of transactions in batch")
	}

	batch.Size = b[0]

	if batch.Size == 0 {
		return batch, errors.New("batch: size must be greater than zero")
	}

	batch.Tags = make([]uint8, batch.Size)
	batch.Payloads = make([][]byte, batch.Size)

	for i := uint8(0); i < batch.Size; i++ {
		if _, err := io.ReadFull(r, b[:1]); err != nil {
			return batch, errors.Wrap(err, "batch: could not read tag")
		}

		if sys.Tag(b[0]) == sys.TagBatch {
			return batch, errors.New("batch: entries inside batch cannot be batch transactions themselves")
		}

		batch.Tags[i] = b[0]

		if _, err := io.ReadFull(r, b[:4]); err != nil {
			return batch, errors.Wrap(err, "batch: could not read payload size")
		}

		size := binary.BigEndian.Uint32(b[:4])
		if size > 2*1024*1024 {
			return batch, errors.New("batch: payload size exceeds 2MB")
		}

		batch.Payloads[i] = make([]byte, size)

		if _, err := io.ReadFull(r, batch.Payloads[i]); err != nil {
			return batch, errors.Wrap(err, "batch: could not read payload")
		}
	}

	return batch, nil
}

func (t Transfer) Marshal() []byte {
	buf := new(bytes.Buffer)
	buf.Write(t.Recipient[:])
	binary.Write(buf, binary.LittleEndian, t.Amount)

	binary.Write(buf, binary.LittleEndian, t.GasLimit)
	binary.Write(buf, binary.LittleEndian, t.GasDeposit)

	if t.FuncName != nil && len(t.FuncName) > 0 {
		binary.Write(buf, binary.LittleEndian, uint32(len(t.FuncName)))
		buf.Write(t.FuncName)

		if t.FuncParams != nil && len(t.FuncParams) > 0 {
			binary.Write(buf, binary.LittleEndian, uint32(len(t.FuncParams)))
			buf.Write(t.FuncParams)
		}
	}

	return buf.Bytes()
}

func (s Stake) Marshal() []byte {
	buf := new(bytes.Buffer)
	buf.WriteByte(s.Opcode)
	binary.Write(buf, binary.LittleEndian, s.Amount)
	return buf.Bytes()
}

func (c Contract) Marshal() []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, c.GasLimit)
	binary.Write(buf, binary.LittleEndian, c.GasDeposit)
	binary.Write(buf, binary.LittleEndian, uint32(len(c.Params)))
	buf.Write(c.Params)
	buf.Write(c.Code)
	return buf.Bytes()
}

// AddTransfer adds a Transfer payload into a batch.
func (b *Batch) AddTransfer(t Transfer) error {
	if b.Size == 255 {
		return fmt.Errorf("batch cannot have more than 255 transactions")
	}

	b.Size++
	b.Tags = append(b.Tags, uint8(sys.TagTransfer))
	b.Payloads = append(b.Payloads, t.Marshal())
	return nil
}

// AddStake adds a Stake payload into a batch.
func (b *Batch) AddStake(s Stake) error {
	if b.Size == 255 {
		return fmt.Errorf("batch cannot have more than 255 transactions")
	}

	b.Size++
	b.Tags = append(b.Tags, uint8(sys.TagStake))
	b.Payloads = append(b.Payloads, s.Marshal())
	return nil

}

// AddContract adds a Contract payload into a batch.
func (b *Batch) AddContract(c Contract) error {
	if b.Size == 255 {
		return fmt.Errorf("batch cannot have more than 255 transactions")
	}

	b.Size++
	b.Tags = append(b.Tags, uint8(sys.TagContract))
	b.Payloads = append(b.Payloads, c.Marshal())
	return nil

}

func (b Batch) Marshal() []byte {
	buf := new(bytes.Buffer)
	buf.WriteByte(byte(b.Size))

	for i := uint8(0); i < b.Size; i++ {
		buf.WriteByte(byte(b.Tags[i]))
		binary.Write(buf, binary.BigEndian, uint32(len(b.Payloads[i])))
		buf.Write(b.Payloads[i])
	}

	return buf.Bytes()
}
