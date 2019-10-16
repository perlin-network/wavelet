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
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"github.com/perlin-network/life/compiler"
	"github.com/perlin-network/life/exec"
	"github.com/perlin-network/life/utils"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/lru"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"reflect"
	"unsafe"
)

var (
	ErrContractFunctionNotFound = errors.New("contract: smart contract func not found")

	_ exec.ImportResolver = (*ContractExecutor)(nil)
	_ compiler.GasPolicy  = (*ContractExecutor)(nil)
)

const (
	PageSize = 65536
)

type ContractExecutor struct {
	ID AccountID

	Gas              uint64
	GasLimitExceeded bool

	Payload []byte
	Error   []byte

	Queue []*Transaction
}

type VMState struct {
	Globals []int64
	Memory  []byte
}

func (state VMState) Apply(vm *exec.VirtualMachine, gasPolicy compiler.GasPolicy, importResolver exec.ImportResolver, move bool) (*exec.VirtualMachine, error) {
	if len(vm.Globals) != len(state.Globals) {
		return nil, errors.New("global count mismatch")
	}

	// safe as `state` is passed by value
	if !move {
		state.Globals = append([]int64{}, state.Globals...)
		state.Memory = append([]byte{}, state.Memory...)
	}

	return &exec.VirtualMachine{
		Config:          vm.Config,
		Module:          vm.Module,
		FunctionCode:    vm.FunctionCode,
		FunctionImports: vm.FunctionImports,
		CallStack:       make([]exec.Frame, exec.DefaultCallStackSize),
		CurrentFrame:    -1,
		Table:           vm.Table,
		Globals:         state.Globals,
		Memory:          state.Memory,
		Exited:          true,
		GasPolicy:       gasPolicy,
		ImportResolver:  importResolver,
	}, nil
}

func CloneVM(vm *exec.VirtualMachine, gasPolicy compiler.GasPolicy, importResolver exec.ImportResolver) (*exec.VirtualMachine, error) {
	return SnapshotVMState(vm).Apply(vm, gasPolicy, importResolver, false)
}

func SnapshotVMState(vm *exec.VirtualMachine) VMState {
	return VMState{
		Globals: vm.Globals,
		Memory:  vm.Memory,
	}
}

func (e *ContractExecutor) GetCost(key string) int64 {
	return 1 // FIXME(kenta): Remove for testnet.
	//cost, ok := sys.GasTable[key]
	//if !ok {
	//	return 1
	//}
	//
	//return int64(cost)
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

				payloadRef := vm.Memory[payloadPtr : payloadPtr+payloadLen]
				payload := make([]byte, len(payloadRef))
				copy(payload, payloadRef)

				e.Queue = append(e.Queue, &Transaction{
					Sender:  e.ID,
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
				//frame := vm.GetCurrentFrame()
				//dataPtr := int(uint32(frame.Locals[0]))
				//dataLen := int(uint32(frame.Locals[1]))
				//
				//logger := log.Contracts("log")
				//logger.Debug().
				//	Hex("contract_id", e.ID[:]).
				//	Msg(string(vm.Memory[dataPtr : dataPtr+dataLen]))

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

// contractState is an optional parameter that is used to pass the VMState of the contract.
// If you cache the VMState, you can pass it.
// If it's nil, we'll try to load the state from the tree.
//
// This function MUST NOT write into the tree. The new or updated VM State must be returned.
func (e *ContractExecutor) Execute(id AccountID, block *Block, tx *Transaction, amount, gasLimit uint64, name string, params, code []byte, tree *avl.Tree, vmCache *lru.LRU, contractState *VMState) (*VMState, error) {
	var vm *exec.VirtualMachine
	var err error

	if cached, ok := vmCache.Load(id); ok {
		vm, err = CloneVM(cached.(*exec.VirtualMachine), e, e)
		if err != nil {
			return nil, errors.Wrap(err, "cannot clone vm")
		}
		vm.Config.GasLimit = gasLimit
	} else {
		config := exec.VMConfig{
			DefaultMemoryPages: sys.ContractDefaultMemoryPages,
			MaxMemoryPages:     sys.ContractMaxMemoryPages,

			DefaultTableSize: sys.ContractTableSize,
			MaxTableSize:     sys.ContractTableSize,

			MaxValueSlots:     sys.ContractMaxValueSlots,
			MaxCallStackDepth: sys.ContractMaxCallStackDepth,
			GasLimit:          gasLimit,
		}

		vm, err = exec.NewVirtualMachine(code, config, e, e)
		if err != nil {
			return nil, errors.Wrap(err, "cannot initialize vm")
		}

		cloned, err := CloneVM(vm, nil, nil)
		if err != nil {
			return nil, errors.Wrap(err, "cannot clone vm")
		}

		vmCache.Put(id, cloned)
	}

	// We can safely initialize the VM first before checking this because the size of the global slice
	// is proportional to the size of the contract's global section.
	if len(vm.Globals) > sys.ContractMaxGlobals {
		return nil, errors.New("too many globals")
	}

	var firstRun bool

	// If state cache is enabled and we have a valid state previously.
	if contractState != nil {
		vm, err = contractState.Apply(vm, e, e, true)
		if err != nil {
			return nil, errors.New("unable to apply state")
		}
	} else if mem := LoadContractMemorySnapshot(tree, id); mem != nil {
		vm.Memory = mem
		if globals, exists := LoadContractGlobals(tree, id); exists {
			if len(globals) == len(vm.Globals) {
				vm.Globals = globals
			}
		}
	} else {
		firstRun = true
	}

	e.ID = id

	e.Payload = buildContractPayload(block, tx, amount, params)

	entry, exists := vm.GetFunctionExport("_contract_" + name)
	if !exists {
		return nil, errors.Wrapf(ErrContractFunctionNotFound, `fn "_contract_%s" does not exist`, name)
	}

	if vm.FunctionCode[entry].NumParams != 0 {
		return nil, errors.New("entry function must not have parameters")
	}

	if firstRun {
		if vm.Module.Base.Start != nil {
			startID := int(vm.Module.Base.Start.Index)

			vm.Ignite(startID)

			for !vm.Exited {
				vm.Execute()

				if vm.Delegate != nil {
					vm.Delegate()
					vm.Delegate = nil
				}
			}
		}
	}

	if vm.ExitError == nil {
		vm.Ignite(entry)

		for !vm.Exited {
			vm.Execute()

			if vm.Delegate != nil {
				vm.Delegate()
				vm.Delegate = nil
			}
		}
	}

	if vm.ExitError != nil {
		fmt.Println("error: ", utils.UnifyError(vm.ExitError))
		vm.PrintStackTrace()
	}

	if vm.ExitError != nil && utils.UnifyError(vm.ExitError).Error() == "gas limit exceeded" {
		e.Gas = gasLimit
		e.GasLimitExceeded = true
	} else {
		e.Gas = vm.Gas
		e.GasLimitExceeded = false
	}

	if vm.ExitError != nil {
		return nil, utils.UnifyError(vm.ExitError)
	}

	vmState := SnapshotVMState(vm)
	return &vmState, nil
}

func LoadContractGlobals(snapshot *avl.Tree, id AccountID) ([]int64, bool) {
	raw, exists := ReadAccountContractGlobals(snapshot, id)
	if !exists {
		return nil, false
	}

	if len(raw)%8 != 0 {
		return nil, false
	}

	// We cannot use the unsafe method as in SaveContractGlobals due to possible alignment issues.
	buf := make([]int64, 0, len(raw)/8)
	for i := 0; i < len(raw); i += 8 {
		buf = append(buf, int64(binary.LittleEndian.Uint64(raw[i:])))
	}
	return buf, true
}

func SaveContractGlobals(snapshot *avl.Tree, id AccountID, globals []int64) {
	oldHeader := (*reflect.SliceHeader)(unsafe.Pointer(&globals))

	header := reflect.SliceHeader{
		Data: oldHeader.Data,
		Len:  oldHeader.Len * 8,
		Cap:  oldHeader.Len * 8, // prevent appending in place
	}
	WriteAccountContractGlobals(snapshot, id, *(*[]byte)(unsafe.Pointer(&header)))
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

	var (
		identical, allZero bool
		pageStart          uint64
	)

	for pageIdx := uint64(0); pageIdx < numPages; pageIdx++ {
		old, _ := ReadAccountContractPage(snapshot, id, pageIdx)
		identical, allZero = true, false
		pageStart = pageIdx * PageSize

		if len(old) == 0 {
			allZero = bytes.Equal(ZeroPage, mem[pageStart:pageStart+PageSize])
			identical = allZero
		} else {
			identical = bytes.Equal(old, mem[pageStart:pageStart+PageSize])
		}

		if !identical {
			// If the page is empty, save an empty byte array. Otherwise, save the pages content.
			if allZero {
				WriteAccountContractPage(snapshot, id, pageIdx, []byte{})
			} else {
				WriteAccountContractPage(snapshot, id, pageIdx, mem[pageStart:pageStart+PageSize])
			}
		}
	}
}

func buildContractPayload(block *Block, tx *Transaction, amount uint64, params []byte) []byte {
	p := make([]byte, 0)
	b := make([]byte, 8)

	var nilAccountID AccountID
	var nilTransactionID TransactionID

	if block != nil {
		binary.LittleEndian.PutUint64(b[:], uint64(block.Index))
	} else {
		binary.LittleEndian.PutUint64(b[:], 0)
	}
	p = append(p, b...)

	if block != nil {
		p = append(p, block.ID[:]...)
	} else {
		p = append(p, nilAccountID[:]...)
	}

	if tx != nil {
		p = append(p, tx.ID[:]...)
		p = append(p, tx.Sender[:]...)
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
