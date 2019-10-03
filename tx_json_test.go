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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseJSON_Transfer(t *testing.T) {
	jsonStr := `{
		"recipient": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
		"amount": 9001,
		"gas_limit": 12345,
		"gas_deposit": 69,
		"fn_name": "hello_world",
		"fn_payload": [
			{"type": "string", "value": "foobar"},
			{"type": "bytes", "value": "loremipsum"},
			{"type": "uint8", "value": 7},
			{"type": "uint16", "value": 42},
			{"type": "uint32", "value": 9001},
			{"type": "uint64", "value": 123456789},
			{"type": "hex", "value": "deadf00d"}
		]
	}`

	b, err := ParseJSON([]byte(jsonStr), "transfer")
	assert.NoError(t, err)

	payload, err := ParseTransfer(b)
	assert.NoError(t, err)
	assert.EqualValues(t, accountID(t, "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405"), payload.Recipient)
	assert.EqualValues(t, 9001, payload.Amount)
	assert.EqualValues(t, 12345, payload.GasLimit)
	assert.EqualValues(t, 69, payload.GasDeposit)
	assert.EqualValues(t, "hello_world", string(payload.FuncName))
	assert.EqualValues(t, []byte{0x6, 0x0, 0x0, 0x0, 0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72, 0xa, 0x0, 0x0, 0x0, 0x6c, 0x6f, 0x72, 0x65, 0x6d, 0x69, 0x70, 0x73, 0x75, 0x6d, 0x7, 0x2a, 0x0, 0x29, 0x23, 0x0, 0x0, 0x15, 0xcd, 0x5b, 0x7, 0x0, 0x0, 0x0, 0x0, 0xde, 0xad, 0xf0, 0xd}, payload.FuncParams)
}

func TestParseJSON_Stake(t *testing.T) {
	jsonStr := `{
		"recipient": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
		"operation": 0,
		"amount": 1337
	}`

	b, err := ParseJSON([]byte(jsonStr), "stake")
	assert.NoError(t, err)

	payload, err := ParseStake(b)
	assert.NoError(t, err)
	assert.EqualValues(t, 1337, payload.Amount)
	assert.EqualValues(t, 0, payload.Opcode)
}

func TestParseJSON_Contract(t *testing.T) {
	jsonStr := `{
		"gas_limit": 12345,
		"gas_deposit": 69,
		"fn_payload": [
			{"type": "string", "value": "foobar"},
			{"type": "bytes", "value": "loremipsum"},
			{"type": "uint8", "value": 7},
			{"type": "uint16", "value": 42},
			{"type": "uint32", "value": 9001},
			{"type": "uint64", "value": 123456789},
			{"type": "hex", "value": "deadf00d"}
		],
		"contract_code": "./testdata/recursive_invocation.wasm"
	}`

	b, err := ParseJSON([]byte(jsonStr), "contract")
	assert.NoError(t, err)

	payload, err := ParseContract(b)
	assert.NoError(t, err)
	assert.EqualValues(t, 12345, payload.GasLimit)
	assert.EqualValues(t, 69, payload.GasDeposit)
	assert.EqualValues(t, []byte{0x6, 0x0, 0x0, 0x0, 0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72, 0xa, 0x0, 0x0, 0x0, 0x6c, 0x6f, 0x72, 0x65, 0x6d, 0x69, 0x70, 0x73, 0x75, 0x6d, 0x7, 0x2a, 0x0, 0x29, 0x23, 0x0, 0x0, 0x15, 0xcd, 0x5b, 0x7, 0x0, 0x0, 0x0, 0x0, 0xde, 0xad, 0xf0, 0xd}, payload.Params)
}

func accountID(t *testing.T, s string) (a AccountID) {
	if hex.DecodedLen(len(s)) != SizeAccountID {
		t.Fatal("wrong SizeAccountID")
	}

	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}

	copy(a[:], b)
	return
}
