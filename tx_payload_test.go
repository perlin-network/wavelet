package wavelet

import (
	"fmt"
	"testing"

	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
)

func TestParseTransfer(t *testing.T) {
	tf := validTransfer(t)
	payload := tf.Marshal()
	tf2, err := ParseTransfer(payload)
	assert.NoError(t, err)
	assert.Equal(t, tf, tf2)

	// FuncParams is optional
	tfNoFuncParams, err := ParseTransfer(payload[:SizeAccountID+8+8+8+4+len(tf.FuncName)])
	assert.NoError(t, err)
	tf.FuncParams = nil
	assert.Equal(t, tf, tfNoFuncParams)

	// FuncName is optional
	tfNoFuncName, err := ParseTransfer(payload[:SizeAccountID+8+8+8])
	assert.NoError(t, err)
	tf.FuncName = nil
	assert.Equal(t, tf, tfNoFuncName)

	// GasDeposit is optional
	tfNoGasDeposit, err := ParseTransfer(payload[:SizeAccountID+8+8])
	assert.NoError(t, err)
	tf.GasDeposit = 0
	assert.Equal(t, tf, tfNoGasDeposit)

	// GasLimit is optional
	tfNoGasLimit, err := ParseTransfer(payload[:SizeAccountID+8])
	assert.NoError(t, err)
	tf.GasLimit = 0
	assert.Equal(t, tf, tfNoGasLimit)
}

func TestParseTransfer_Errors(t *testing.T) {
	tests := []struct {
		Err     string
		Payload func(tf Transfer) []byte
	}{
		{
			"failed to decode recipient",
			func(tf Transfer) []byte {
				payload := tf.Marshal()
				return payload[:SizeAccountID-1]
			},
		},
		{
			"failed to decode amount of PERLs to send",
			func(tf Transfer) []byte {
				payload := tf.Marshal()
				return payload[:SizeAccountID+7]
			},
		},
		{
			"failed to decode gas limit",
			func(tf Transfer) []byte {
				payload := tf.Marshal()
				return payload[:SizeAccountID+8+7]
			},
		},
		{
			"failed to decode gas deposit",
			func(tf Transfer) []byte {
				payload := tf.Marshal()
				return payload[:SizeAccountID+8+8+7]
			},
		},
		{
			"failed to decode size of smart contract function name to invoke",
			func(tf Transfer) []byte {
				payload := tf.Marshal()
				return payload[:SizeAccountID+8+8+8+3]
			},
		},
		{
			"gas limit for invoking smart contract function must be greater than zero",
			func(tf Transfer) []byte {
				tf.GasLimit = 0
				return tf.Marshal()
			},
		},
		{
			"smart contract function name exceeds 1024 characters",
			func(tf Transfer) []byte {
				tf.FuncName = make([]byte, 1025)
				return tf.Marshal()
			},
		},
		{
			"failed to decode smart contract function name to invoke",
			func(tf Transfer) []byte {
				payload := tf.Marshal()
				return payload[:SizeAccountID+8+8+8+4+len(tf.FuncName)-1]
			},
		},
		{
			"not allowed to call init function for smart contract",
			func(tf Transfer) []byte {
				tf.FuncName = []byte("init")
				return tf.Marshal()
			},
		},
		{
			"failed to decode number of smart contract function invocation parameters",
			func(tf Transfer) []byte {
				payload := tf.Marshal()
				return payload[:SizeAccountID+8+8+8+4+len(tf.FuncName)+3]
			},
		},
		{
			"smart contract payload exceeds 1MB",
			func(tf Transfer) []byte {
				tf.FuncParams = make([]byte, (1024*1024)+1)
				return tf.Marshal()
			},
		},
		{
			"failed to decode smart contract function invocation parameters",
			func(tf Transfer) []byte {
				payload := tf.Marshal()
				return payload[:SizeAccountID+8+8+8+4+len(tf.FuncName)+4+len(tf.FuncParams)-1]
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Err, func(t *testing.T) {
			_, err := ParseTransfer(tt.Payload(validTransfer(t)))
			if err == nil {
				t.Fatal("expecting an error, got nil instead")
			}
			assert.Contains(t, err.Error(), fmt.Sprintf("transfer: %s", tt.Err))
		})
	}
}

func TestParseStake(t *testing.T) {
	stake := validStake(t, sys.WithdrawStake)
	payload := stake.Marshal()

	stake2, err := ParseStake(payload)
	assert.NoError(t, err)
	assert.Equal(t, stake, stake2)

	// PlaceStake and WithdrawStake don't have minimum stake amount
	stake.Amount = 1
	stake.Opcode = sys.PlaceStake
	stakePlace, err := ParseStake(stake.Marshal())
	assert.NoError(t, err)
	assert.Equal(t, stake, stakePlace)

	stake.Opcode = sys.WithdrawStake
	stakeWithdraw, err := ParseStake(stake.Marshal())
	assert.NoError(t, err)
	assert.Equal(t, stake, stakeWithdraw)
}

func TestParseStake_Errors(t *testing.T) {
	tests := []struct {
		Err     string
		Payload func() []byte
	}{
		{
			"payload must be exactly 9 bytes",
			func() []byte {
				payload := validStake(t, sys.WithdrawReward).Marshal()
				return payload[:len(payload)-1]
			},
		},
		{
			"opcode must be 0, 1, or 2",
			func() []byte {
				return validStake(t, sys.WithdrawReward+1).Marshal()
			},
		},
		{
			"amount must be greater than zero",
			func() []byte {
				stake := validStake(t, sys.WithdrawReward)
				stake.Amount = 0
				return stake.Marshal()
			},
		},
		{
			"must withdraw a reward of a minimum of 100 PERLs, but requested to withdraw 1 PERLs",
			func() []byte {
				stake := validStake(t, sys.WithdrawReward)
				stake.Amount = 1
				return stake.Marshal()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Err, func(t *testing.T) {
			_, err := ParseStake(tt.Payload())
			if err == nil {
				t.Fatal("expecting an error, got nil instead")
			}
			assert.Contains(t, err.Error(), fmt.Sprintf("stake: %s", tt.Err))
		})
	}
}

func TestParseContract(t *testing.T) {
	contract := validContract(t)
	payload := contract.Marshal()

	contract2, err := ParseContract(payload)
	assert.NoError(t, err)
	assert.Equal(t, contract, contract2)
}

func TestParseContract_Errors(t *testing.T) {
	tests := []struct {
		Err     string
		Payload func() []byte
	}{
		{
			"failed to decode gas limit",
			func() []byte {
				payload := validContract(t).Marshal()
				return payload[:7]
			},
		},
		{
			"failed to decode gas deposit",
			func() []byte {
				payload := validContract(t).Marshal()
				return payload[:8+7]
			},
		},

		{
			"failed to decode number of smart contract init parameters",
			func() []byte {
				payload := validContract(t).Marshal()
				return payload[:8+8+3]
			},
		},
		{
			"smart contract payload exceeds 1MB",
			func() []byte {
				contract := validContract(t)
				contract.Params = make([]byte, (1024*1024)+1)
				return contract.Marshal()
			},
		},
		{
			"gas limit for invoking smart contract function must be greater than zero",
			func() []byte {
				contract := validContract(t)
				contract.GasLimit = 0
				return contract.Marshal()
			},
		},
		{
			"failed to decode smart contract init parameters",
			func() []byte {
				contract := validContract(t)
				payload := contract.Marshal()
				return payload[:8+8+4+len(contract.Params)-1]
			},
		},
		{
			"smart contract must have code of length greater than zero",
			func() []byte {
				contract := validContract(t)
				contract.Code = []byte{}
				return contract.Marshal()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Err, func(t *testing.T) {
			_, err := ParseContract(tt.Payload())
			if err == nil {
				t.Fatal("expecting an error, got nil instead")
			}
			assert.Contains(t, err.Error(), fmt.Sprintf("contract: %s", tt.Err))
		})
	}
}

func TestParseBatch(t *testing.T) {
	batch := validBatch(t)
	payload := batch.Marshal()

	batch2, err := ParseBatch(payload)
	assert.NoError(t, err)
	assert.Equal(t, batch, batch2)
}

func TestParseBatch_Errors(t *testing.T) {
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
				return (Batch{}).Marshal()
			},
		},
		{
			"could not read tag",
			func() []byte {
				payload := validBatch(t).Marshal()
				return payload[:1]
			},
		},
		{
			"entries inside batch cannot be batch transactions themselves",
			func() []byte {
				batch := validBatch(t)
				batch.Tags[0] = uint8(sys.TagBatch)
				batch.Payloads[0] = batch.Marshal()

				return batch.Marshal()
			},
		},
		{
			"could not read payload size",
			func() []byte {
				payload := validBatch(t).Marshal()
				return payload[:1+1+3]
			},
		},
		{
			"payload size exceeds 2MB",
			func() []byte {
				batch := validBatch(t)
				batch.Payloads[0] = make([]byte, 2*1024*1024+1)

				return batch.Marshal()
			},
		},
		{
			"could not read payload",
			func() []byte {
				payload := validBatch(t).Marshal()
				return payload[:1+1+4+1]
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Err, func(t *testing.T) {
			_, err := ParseBatch(tt.Payload())
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
	var batch Batch
	assert.NoError(t, batch.AddTransfer(validTransfer(t)))
	assert.NoError(t, batch.AddStake(validStake(t, sys.PlaceStake)))
	assert.NoError(t, batch.AddContract(validContract(t)))
	return batch
}
