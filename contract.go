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
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"github.com/perlin-network/life/compiler"
	"github.com/perlin-network/life/exec"
	"github.com/perlin-network/life/utils"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

var (
	ErrNotSmartContract         = errors.New("contract: specified account ID is not a smart contract")
	ErrContractFunctionNotFound = errors.New("contract: smart contract func not found")

	_ exec.ImportResolver = (*ContractExecutor)(nil)
	_ compiler.GasPolicy  = (*ContractExecutor)(nil)
)

const (
	PageSize = 65536
)

type ContractExecutor struct {
	ID       AccountID
	Snapshot *avl.Tree

	Gas              uint64
	GasLimitExceeded bool

	Payload []byte
	Error   []byte

	Queue []*Transaction
}

func (e *ContractExecutor) GetCost(key string) int64 {
	cost, ok := sys.GasTable[key]
	if !ok {
		return 1
	}

	return int64(cost)
}

func (e *ContractExecutor) ResolveFunc(module, field string) exec.FunctionImport {
	switch module {
	case "env":
		switch field {
		case "abort":
			return func(vm *exec.VirtualMachine) int64 {
				panic(errors.New("abort called"))
			}
		case "_send_transaction":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				tag := byte(uint32(frame.Locals[0]))
				payloadPtr := int(uint32(frame.Locals[1]))
				payloadLen := int(uint32(frame.Locals[2]))

				payload := vm.Memory[payloadPtr : payloadPtr+payloadLen]

				e.Queue = append(e.Queue, &Transaction{
					Sender:  e.ID,
					Creator: e.ID,
					Tag:     sys.Tag(tag),
					Payload: payload,
				})

				return 0
			}
		case "_payload_len":
			return func(vm *exec.VirtualMachine) int64 {
				return int64(len(e.Payload))
			}
		case "_payload":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				outPtr := int(uint32(frame.Locals[0]))
				copy(vm.Memory[outPtr:], e.Payload)
				return 0
			}
		case "_result":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				dataPtr := int(uint32(frame.Locals[0]))
				dataLen := int(uint32(frame.Locals[1]))

				e.Error = make([]byte, dataLen)
				copy(e.Error, vm.Memory[dataPtr:dataPtr+dataLen])
				return 0
			}
		case "_log":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				dataPtr := int(uint32(frame.Locals[0]))
				dataLen := int(uint32(frame.Locals[1]))

				logger := log.Contracts("log")
				logger.Debug().
					Hex("contract_id", e.ID[:]).
					Msg(string(vm.Memory[dataPtr : dataPtr+dataLen]))

				return 0
			}
		case "_verify_ed25519":
			return func(vm *exec.VirtualMachine) int64 {
				vm.Gas += uint64(e.GetCost("wavelet.verify.ed25519"))

				frame := vm.GetCurrentFrame()
				keyPtr, keyLen := int(uint32(frame.Locals[0])), int(uint32(frame.Locals[1]))
				dataPtr, dataLen := int(uint32(frame.Locals[2])), int(uint32(frame.Locals[3]))
				sigPtr, sigLen := int(uint32(frame.Locals[4])), int(uint32(frame.Locals[5]))

				if keyLen != edwards25519.SizePublicKey || sigLen != edwards25519.SizeSignature {
					return 1
				}

				key := vm.Memory[keyPtr : keyPtr+keyLen]
				data := vm.Memory[dataPtr : dataPtr+dataLen]
				sig := vm.Memory[sigPtr : sigPtr+sigLen]

				var pub edwards25519.PublicKey
				var edSig edwards25519.Signature

				copy(pub[:], key)
				copy(edSig[:], sig)

				ok := edwards25519.Verify(pub, data, edSig)
				if ok {
					return 0
				} else {
					return 1
				}
			}
		case "_hash_blake2b_256":
			return buildHashImpl(
				uint64(e.GetCost("wavelet.hash.blake2b256")),
				blake2b.Size256,
				func(data, out []byte) {
					b := blake2b.Sum256(data)
					copy(out, b[:])
				},
			)
		case "_hash_blake2b_512":
			return buildHashImpl(
				uint64(e.GetCost("wavelet.hash.blake2b512")),
				blake2b.Size,
				func(data, out []byte) {
					b := blake2b.Sum512(data)
					copy(out, b[:])
				},
			)
		case "_hash_sha256":
			return buildHashImpl(
				uint64(e.GetCost("wavelet.hash.sha256")),
				sha256.Size,
				func(data, out []byte) {
					b := sha256.Sum256(data)
					copy(out, b[:])
				},
			)
		case "_hash_sha512":
			return buildHashImpl(
				uint64(e.GetCost("wavelet.hash.sha512")),
				sha512.Size,
				func(data, out []byte) {
					b := sha512.Sum512(data)
					copy(out, b[:])
				},
			)
		default:
			panic("unknown field")
		}
	default:
		panic("unknown module")
	}
}

func (e *ContractExecutor) ResolveGlobal(module, field string) int64 {
	panic("global variables are disallowed in smart contracts")
}

func (e *ContractExecutor) Execute(snapshot *avl.Tree, id AccountID, round *Round, tx *Transaction, amount, gasLimit uint64, name string, params, code []byte) error {
	config := exec.VMConfig{
		DefaultMemoryPages: 4,
		MaxMemoryPages:     32,

		DefaultTableSize: PageSize,
		MaxTableSize:     PageSize,

		MaxValueSlots:     4096,
		MaxCallStackDepth: 256,
		GasLimit:          gasLimit,
	}

	vm, err := exec.NewVirtualMachine(code, config, e, e)
	if err != nil {
		return errors.Wrap(err, "could not init vm")
	}

	if mem := LoadContractMemorySnapshot(snapshot, id); mem != nil {
		vm.Memory = mem
	}

	e.ID = id
	e.Snapshot = snapshot

	e.Payload = buildContractPayload(round, tx, amount, params)

	entry, exists := vm.GetFunctionExport("_contract_" + name)
	if !exists {
		return errors.Wrapf(ErrContractFunctionNotFound, `fn "_contract_%s" does not exist`, name)
	}

	if vm.FunctionCode[entry].NumParams != 0 {
		return errors.New("entry function must not have parameters")
	}

	vm.Ignite(entry)

	for !vm.Exited {
		vm.Execute()

		if vm.Delegate != nil {
			vm.Delegate()
			vm.Delegate = nil
		}
	}

	if vm.ExitError == nil && len(e.Error) == 0 {
		SaveContractMemorySnapshot(snapshot, id, vm.Memory)
	}

	if vm.ExitError != nil && utils.UnifyError(vm.ExitError).Error() == "gas limit exceeded" {
		e.Gas = gasLimit
		e.GasLimitExceeded = true
	} else {
		e.Gas = vm.Gas
		e.GasLimitExceeded = false
	}

	return nil
}

func LoadContractMemorySnapshot(snapshot *avl.Tree, id AccountID) []byte {
	numPages, exists := ReadAccountContractNumPages(snapshot, id)
	if !exists {
		return nil
	}

	mem := make([]byte, PageSize*numPages)

	for pageIdx := uint64(0); pageIdx < numPages; pageIdx++ {
		page, _ := ReadAccountContractPage(snapshot, id, pageIdx)

		if len(page) > 0 {
			copy(mem[PageSize*pageIdx:PageSize*(pageIdx+1)], page)
		}
	}

	return mem
}

func SaveContractMemorySnapshot(snapshot *avl.Tree, id AccountID, mem []byte) {
	numPages := uint64(len(mem) / PageSize)

	WriteAccountContractNumPages(snapshot, id, numPages)

	for pageIdx := uint64(0); pageIdx < numPages; pageIdx++ {
		old, _ := ReadAccountContractPage(snapshot, id, pageIdx)

		identical := true

		for idx := uint64(0); idx < PageSize; idx++ {
			if len(old) == 0 && mem[pageIdx*PageSize+idx] != 0 {
				identical = false
				break
			}

			if len(old) != 0 && mem[pageIdx*PageSize+idx] != old[idx] {
				identical = false
				break
			}
		}

		if !identical {
			allZero := true

			for idx := uint64(0); idx < PageSize; idx++ {
				if mem[pageIdx*PageSize+idx] != 0 {
					allZero = false
					break
				}
			}

			// If the page is empty, save an empty byte array. Otherwise, save the pages content.

			if allZero {
				WriteAccountContractPage(snapshot, id, pageIdx, []byte{})
			} else {
				WriteAccountContractPage(snapshot, id, pageIdx, mem[pageIdx*PageSize:(pageIdx+1)*PageSize])
			}
		}
	}
}

func buildContractPayload(round *Round, tx *Transaction, amount uint64, params []byte) []byte {
	p := make([]byte, 0)
	b := make([]byte, 8)

	var nilAccountID AccountID
	var nilTransactionID TransactionID

	if round != nil {
		binary.LittleEndian.PutUint64(b[:], uint64(round.Index))
	} else {
		binary.LittleEndian.PutUint64(b[:], 0)
	}
	p = append(p, b...)

	if round != nil {
		p = append(p, round.ID[:]...)
	} else {
		p = append(p, nilAccountID[:]...)
	}

	if tx != nil {
		p = append(p, tx.ID[:]...)
		p = append(p, tx.Creator[:]...)
	} else {
		p = append(p, nilTransactionID[:]...)
		p = append(p, nilAccountID[:]...)
	}

	binary.LittleEndian.PutUint64(b[:], amount)
	p = append(p, b...)

	p = append(p, params...)

	return p
}

func buildHashImpl(gas uint64, size int, f func(data, out []byte)) func(vm *exec.VirtualMachine) int64 {
	return func(vm *exec.VirtualMachine) int64 {
		vm.Gas += gas

		frame := vm.GetCurrentFrame()
		dataPtr, dataLen := int(uint32(frame.Locals[0])), int(uint32(frame.Locals[1]))
		outPtr, outLen := int(uint32(frame.Locals[2])), int(uint32(frame.Locals[3]))
		if outLen != size {
			return 1
		}

		data := vm.Memory[dataPtr : dataPtr+dataLen]
		out := vm.Memory[outPtr : outPtr+outLen]
		f(data, out)
		return 0
	}
}
