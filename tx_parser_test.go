package wavelet

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
)

func TestParseTransferTransaction(t *testing.T) {
	tx := validTransfer(t)
	payload := encodeTransfer(tx)
	tx2, err := ParseTransferTransaction(payload)
	assert.NoError(t, err)
	assert.Equal(t, tx, tx2)

	// FuncParams is optional
	txNoFuncParams, err := ParseTransferTransaction(payload[:SizeAccountID+8+8+8+4+len(tx.FuncName)])
	assert.NoError(t, err)
	tx.FuncParams = nil
	assert.Equal(t, tx, txNoFuncParams)

	// FuncName is optional
	txNoFuncName, err := ParseTransferTransaction(payload[:SizeAccountID+8+8+8])
	assert.NoError(t, err)
	tx.FuncName = nil
	assert.Equal(t, tx, txNoFuncName)

	// GasDeposit is optional
	txNoGasDeposit, err := ParseTransferTransaction(payload[:SizeAccountID+8+8])
	assert.NoError(t, err)
	tx.GasDeposit = 0
	assert.Equal(t, tx, txNoGasDeposit)

	// GasLimit is optional
	txNoGasLimit, err := ParseTransferTransaction(payload[:SizeAccountID+8])
	assert.NoError(t, err)
	tx.GasLimit = 0
	assert.Equal(t, tx, txNoGasLimit)
}

func TestParseTransferTransaction_Errors(t *testing.T) {
	tests := []struct {
		Err     string
		Payload func(tx Transfer) []byte
	}{
		{
			"failed to decode recipient",
			func(tx Transfer) []byte {
				payload := encodeTransfer(tx)
				return payload[:SizeAccountID-1]
			},
		},
		{
			"failed to decode amount of PERLs to send",
			func(tx Transfer) []byte {
				payload := encodeTransfer(tx)
				return payload[:SizeAccountID+7]
			},
		},
		{
			"failed to decode gas limit",
			func(tx Transfer) []byte {
				payload := encodeTransfer(tx)
				return payload[:SizeAccountID+8+7]
			},
		},
		{
			"failed to decode gas deposit",
			func(tx Transfer) []byte {
				payload := encodeTransfer(tx)
				return payload[:SizeAccountID+8+8+7]
			},
		},

		{
			"failed to decode size of smart contract function name to invoke",
			func(tx Transfer) []byte {
				payload := encodeTransfer(tx)
				return payload[:SizeAccountID+8+8+8+3]
			},
		},
		{
			"smart contract function name exceeds 1024 characters",
			func(tx Transfer) []byte {
				tx.FuncName = make([]byte, 1025)
				return encodeTransfer(tx)
			},
		},
		{
			"failed to decode smart contract function name to invoke",
			func(tx Transfer) []byte {
				payload := encodeTransfer(tx)
				return payload[:SizeAccountID+8+8+8+4+len(tx.FuncName)-1]
			},
		},
		{
			"not allowed to call init function for smart contract",
			func(tx Transfer) []byte {
				tx.FuncName = []byte("init")
				return encodeTransfer(tx)
			},
		},
		{
			"failed to decode number of smart contract function invocation parameters",
			func(tx Transfer) []byte {
				payload := encodeTransfer(tx)
				return payload[:SizeAccountID+8+8+8+4+len(tx.FuncName)+3]
			},
		},
		{
			"smart contract payload exceeds 1MB",
			func(tx Transfer) []byte {
				tx.FuncParams = make([]byte, (1024*1024)+1)
				return encodeTransfer(tx)
			},
		},
		{
			"failed to decode smart contract function invocation parameters",
			func(tx Transfer) []byte {
				payload := encodeTransfer(tx)
				return payload[:SizeAccountID+8+8+8+4+len(tx.FuncName)+4+len(tx.FuncParams)-1]
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Err, func(t *testing.T) {
			_, err := ParseTransferTransaction(tt.Payload(validTransfer(t)))
			if err == nil {
				t.Fatal("expecting an error, got nil instead")
			}
			assert.Contains(t, err.Error(), fmt.Sprintf("transfer: %s", tt.Err))
		})
	}
}

func TestParseStakeTransaction(t *testing.T) {
	stake := validStake(t, sys.WithdrawStake)
	payload := encodeStake(stake)

	stake2, err := ParseStakeTransaction(payload)
	assert.NoError(t, err)
	assert.Equal(t, stake, stake2)

	// PlaceStake and WithdrawStake don't have minimum stake amount
	stake.Amount = 1
	stake.Opcode = sys.PlaceStake
	stakePlace, err := ParseStakeTransaction(encodeStake(stake))
	assert.NoError(t, err)
	assert.Equal(t, stake, stakePlace)

	stake.Opcode = sys.WithdrawStake
	stakeWithdraw, err := ParseStakeTransaction(encodeStake(stake))
	assert.NoError(t, err)
	assert.Equal(t, stake, stakeWithdraw)
}

func TestParseStakeTransaction_Errors(t *testing.T) {
	tests := []struct {
		Err     string
		Payload func() []byte
	}{
		{
			"payload must be exactly 9 bytes",
			func() []byte {
				payload := encodeStake(validStake(t, sys.WithdrawReward))
				return payload[:len(payload)-1]
			},
		},
		{
			"opcode must be 0, 1, or 2",
			func() []byte {
				return encodeStake(validStake(t, sys.WithdrawReward+1))
			},
		},
		{
			"amount must be greater than zero",
			func() []byte {
				stake := validStake(t, sys.WithdrawReward)
				stake.Amount = 0
				return encodeStake(stake)
			},
		},
		{
			"must withdraw a reward of a minimum of 100 PERLs, but requested to withdraw 1 PERLs",
			func() []byte {
				stake := validStake(t, sys.WithdrawReward)
				stake.Amount = 1
				return encodeStake(stake)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Err, func(t *testing.T) {
			_, err := ParseStakeTransaction(tt.Payload())
			if err == nil {
				t.Fatal("expecting an error, got nil instead")
			}
			assert.Contains(t, err.Error(), fmt.Sprintf("stake: %s", tt.Err))
		})
	}
}

func TestParseContractTransaction(t *testing.T) {
	contract := validContract(t)
	payload := encodeContract(contract)

	contract2, err := ParseContractTransaction(payload)
	assert.NoError(t, err)
	assert.Equal(t, contract, contract2)
}

func TestParseContractTransaction_Errors(t *testing.T) {
	tests := []struct {
		Err     string
		Payload func() []byte
	}{
		{
			"failed to decode gas limit",
			func() []byte {
				payload := encodeContract(validContract(t))
				return payload[:7]
			},
		},
		{
			"failed to decode gas deposit",
			func() []byte {
				payload := encodeContract(validContract(t))
				return payload[:8+7]
			},
		},

		{
			"failed to decode number of smart contract init parameters",
			func() []byte {
				payload := encodeContract(validContract(t))
				return payload[:8+8+3]
			},
		},
		{
			"smart contract payload exceeds 1MB",
			func() []byte {
				contract := validContract(t)
				contract.Params = make([]byte, (1024*1024)+1)
				return encodeContract(contract)
			},
		},

		{
			"failed to decode smart contract init parameters",
			func() []byte {
				contract := validContract(t)
				payload := encodeContract(contract)
				return payload[:8+8+4+len(contract.Params)-1]
			},
		},
		{
			"smart contract must have code of length greater than zero",
			func() []byte {
				contract := validContract(t)
				contract.Code = []byte{}
				return encodeContract(contract)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Err, func(t *testing.T) {
			_, err := ParseContractTransaction(tt.Payload())
			if err == nil {
				t.Fatal("expecting an error, got nil instead")
			}
			assert.Contains(t, err.Error(), fmt.Sprintf("contract: %s", tt.Err))
		})
	}
}

func TestParseBatchTransaction(t *testing.T) {
	batch := validBatch(t)
	payload := encodeBatch(batch)

	batch2, err := ParseBatchTransaction(payload)
	assert.NoError(t, err)
	assert.Equal(t, batch, batch2)
}

func TestParseBatchTransaction_Errors(t *testing.T) {
	tests := []struct {
		Err     string
		Payload func() []byte
	}{
		{
			"failed to decode number of transactions in batch",
			func() []byte {
				return []byte{}
			},
		},
		{
			"size must be greater than zero",
			func() []byte {
				return encodeBatch(Batch{})
			},
		},
		{
			"could not read tag",
			func() []byte {
				payload := encodeBatch(validBatch(t))
				return payload[:1]
			},
		},
		{
			"entries inside batch cannot be batch transactions themselves",
			func() []byte {
				batch := validBatch(t)
				batch.Tags[0] = uint8(sys.TagBatch)
				batch.Payloads[0] = encodeBatch(batch)

				return encodeBatch(batch)
			},
		},
		{
			"could not read payload size",
			func() []byte {
				payload := encodeBatch(validBatch(t))
				return payload[:1+1+3]
			},
		},
		{
			"payload size exceeds 2MB",
			func() []byte {
				batch := validBatch(t)
				batch.Payloads[0] = make([]byte, 2*1024*1024+1)

				return encodeBatch(batch)
			},
		},
		{
			"could not read payload",
			func() []byte {
				payload := encodeBatch(validBatch(t))
				return payload[:1+1+4+1]
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Err, func(t *testing.T) {
			_, err := ParseBatchTransaction(tt.Payload())
			if err == nil {
				t.Fatal("expecting an error, got nil instead")
			}
			assert.Contains(t, err.Error(), fmt.Sprintf("batch: %s", tt.Err))
		})
	}
}

func validTransfer(t *testing.T) Transfer {
	keys, err := skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		t.Fatal(err)
	}

	return Transfer{
		Recipient:  keys.PublicKey(),
		Amount:     1337,
		GasLimit:   42,
		GasDeposit: 10,
		FuncName:   []byte("helloworld"),
		FuncParams: []byte("foobar"),
	}
}

func validStake(t *testing.T, opcode byte) Stake {
	return Stake{
		Opcode: opcode,
		Amount: uint64(1337),
	}
}

func validContract(t *testing.T) Contract {
	return Contract{
		GasLimit:   42,
		GasDeposit: 10,
		Params:     []byte("foobar"),
		Code:       []byte("loremipsumdolorsitamet"),
	}
}

func validBatch(t *testing.T) Batch {
	return Batch{
		Size: 3,
		Tags: []uint8{
			uint8(sys.TagTransfer), uint8(sys.TagStake), uint8(sys.TagContract),
		},
		Payloads: [][]byte{
			encodeTransfer(validTransfer(t)),
			encodeStake(validStake(t, sys.PlaceStake)),
			encodeContract(validContract(t)),
		},
	}
}

func encodeTransfer(tx Transfer) []byte {
	buf := new(bytes.Buffer)
	buf.Write(tx.Recipient[:])
	binary.Write(buf, binary.LittleEndian, tx.Amount)
	binary.Write(buf, binary.LittleEndian, tx.GasLimit)
	binary.Write(buf, binary.LittleEndian, tx.GasDeposit)

	binary.Write(buf, binary.LittleEndian, uint32(len(tx.FuncName)))
	buf.Write(tx.FuncName)

	binary.Write(buf, binary.LittleEndian, uint32(len(tx.FuncParams)))
	buf.Write(tx.FuncParams)

	return buf.Bytes()
}

func encodeStake(s Stake) []byte {
	buf := new(bytes.Buffer)
	buf.WriteByte(s.Opcode)
	binary.Write(buf, binary.LittleEndian, s.Amount)
	return buf.Bytes()
}

func encodeContract(c Contract) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, c.GasLimit)
	binary.Write(buf, binary.LittleEndian, c.GasDeposit)
	binary.Write(buf, binary.LittleEndian, uint32(len(c.Params)))
	buf.Write(c.Params)
	buf.Write(c.Code)
	return buf.Bytes()
}

func encodeBatch(b Batch) []byte {
	buf := new(bytes.Buffer)
	buf.WriteByte(byte(b.Size))

	for i := uint8(0); i < b.Size; i++ {
		buf.WriteByte(byte(b.Tags[i]))
		binary.Write(buf, binary.BigEndian, uint32(len(b.Payloads[i])))
		buf.Write(b.Payloads[i])
	}

	return buf.Bytes()
}
