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
	"encoding/binary"
	"github.com/perlin-network/life/compiler"
	"github.com/perlin-network/life/exec"
	"github.com/perlin-network/life/utils"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

var (
	ErrNotSmartContract         = errors.New("contract: specified account id is not a smart contract")
	ErrContractFunctionNotFound = errors.New("contract: smart contract func not found")

	_ exec.ImportResolver = (*ContractExecutor)(nil)
	_ compiler.GasPolicy  = (*ContractExecutor)(nil)
)

const (
	PageSize = 65536
)

type ContractExecutor struct {
	contractID AccountID
	ctx        *TransactionContext

	gasTable       map[string]uint64
	header, result []byte

	enableLogging bool
}

func NewContractExecutor(contractID AccountID, ctx *TransactionContext) *ContractExecutor {
	return &ContractExecutor{contractID: contractID, ctx: ctx}
}

func (c *ContractExecutor) WithGasTable(gasTable map[string]uint64) *ContractExecutor {
	c.gasTable = gasTable
	return c
}

func (c *ContractExecutor) EnableLogging() *ContractExecutor {
	c.enableLogging = true
	return c
}

func (c *ContractExecutor) LoadMemorySnapshot() ([]byte, error) {
	numPages, exists := c.ctx.ReadAccountContractNumPages(c.contractID)
	if !exists {
		return nil, nil
	}

	mem := make([]byte, PageSize*numPages)

	for pageIdx := uint64(0); pageIdx < numPages; pageIdx++ {
		page, _ := c.ctx.ReadAccountContractPage(c.contractID, pageIdx)

		// Zero-filled by default.
		if len(page) > 0 {
			if len(page) != PageSize {
				return nil, errors.Errorf("page %d has a size of %d != PageSize", pageIdx, len(page))
			}

			copy(mem[PageSize*pageIdx:PageSize*(pageIdx+1)], page)
		}
	}

	return mem, nil
}

func (c *ContractExecutor) SaveMemorySnapshot(contractID AccountID, memory []byte) {
	numPages := uint64(len(memory) / PageSize)

	c.ctx.WriteAccountContractNumPages(contractID, numPages)

	for pageIdx := uint64(0); pageIdx < numPages; pageIdx++ {
		old, _ := c.ctx.ReadAccountContractPage(contractID, pageIdx)

		identical := true

		for idx := uint64(0); idx < PageSize; idx++ {
			if len(old) == 0 && memory[pageIdx*PageSize+idx] != 0 {
				identical = false
				break
			}

			if len(old) != 0 && memory[pageIdx*PageSize+idx] != old[idx] {
				identical = false
				break
			}
		}

		if !identical {
			allZero := true

			for idx := uint64(0); idx < PageSize; idx++ {
				if memory[pageIdx*PageSize+idx] != 0 {
					allZero = false
					break
				}
			}

			// If the page is empty, save an empty byte array. Otherwise, save the pages content.
			if allZero {
				c.ctx.WriteAccountContractPage(contractID, pageIdx, []byte{})
			} else {
				c.ctx.WriteAccountContractPage(contractID, pageIdx, memory[pageIdx*PageSize:(pageIdx+1)*PageSize])
			}
		}
	}
}

func (c *ContractExecutor) Init(code []byte, gasLimit uint64) (*exec.VirtualMachine, error) {
	config := exec.VMConfig{
		DefaultMemoryPages: 16,
		MaxMemoryPages:     32,

		DefaultTableSize: PageSize,
		MaxTableSize:     PageSize,

		MaxValueSlots:     4096,
		MaxCallStackDepth: 256,
		GasLimit:          gasLimit,
	}

	vm, err := exec.NewVirtualMachine(code, config, c, c)
	if err != nil {
		return nil, errors.Wrap(err, "contract: failed to init smart contract vm")
	}

	return vm, nil
}

func (c *ContractExecutor) Run(amount, gasLimit uint64, entry string, payload ...byte) ([]byte, uint64, error) {
	tx := c.ctx.Transaction()

	code, available := c.ctx.ReadAccountContractCode(c.contractID)
	if !available {
		return nil, 0, ErrNotSmartContract
	}

	vm, err := c.Init(code, gasLimit)
	if err != nil {
		return nil, 0, err
	}

	// Load memory snapshot if available.
	mem, err := c.LoadMemorySnapshot()
	if err != nil {
		return nil, 0, errors.Wrap(err, "contract: failed to load memory snapshot")
	}

	if mem != nil {
		vm.Memory = mem
	}

	c.header = make([]byte, SizeTransactionID+SizeAccountID+8+len(payload))

	copy(c.header[0:SizeTransactionID], tx.ID[:])
	copy(c.header[SizeTransactionID:SizeTransactionID+SizeAccountID], tx.Creator[:])

	binary.LittleEndian.PutUint64(c.header[SizeTransactionID+SizeAccountID:8+SizeTransactionID+SizeAccountID], amount)

	copy(c.header[8+SizeTransactionID+SizeAccountID:8+SizeTransactionID+SizeAccountID+len(payload)], payload)

	entry = "_contract_" + entry

	entryID, exists := vm.GetFunctionExport(entry)
	if !exists {
		return nil, vm.Gas, errors.Wrapf(ErrContractFunctionNotFound, "`%s` does not exist", entry)
	}

	// Execute virtual machine.
	vm.Ignite(entryID)

	for !vm.Exited {
		vm.Execute()

		if vm.Delegate != nil {
			vm.Delegate()
			vm.Delegate = nil
		}
	}

	if vm.ExitError != nil {
		return nil, vm.Gas, utils.UnifyError(vm.ExitError)
	}

	// Save memory snapshot.
	c.SaveMemorySnapshot(c.contractID, vm.Memory)

	return c.result, vm.Gas, nil
}

func (c *ContractExecutor) GetCost(key string) int64 {
	if c.gasTable == nil {
		return 1
	}

	cost, ok := c.gasTable[key]

	if !ok {
		return 1
	}

	return int64(cost)
}

func (c *ContractExecutor) ResolveFunc(module, field string) exec.FunctionImport {
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

				inputs := vm.Memory[payloadPtr : payloadPtr+payloadLen]

				c.ctx.transactions.PushBack(&Transaction{
					Sender:  c.contractID,
					Creator: c.contractID,
					Tag:     tag,
					Payload: inputs,
				})

				return 0
			}
		case "_payload_len":
			return func(vm *exec.VirtualMachine) int64 {
				return int64(len(c.header))
			}
		case "_payload":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				outPtr := int(uint32(frame.Locals[0]))
				copy(vm.Memory[outPtr:], c.header)
				return 0
			}
		case "_result":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				dataPtr := int(uint32(frame.Locals[0]))
				dataLen := int(uint32(frame.Locals[1]))

				c.result = make([]byte, dataLen)
				copy(c.result, vm.Memory[dataPtr:dataPtr+dataLen])
				return 0
			}
		case "_log":
			return func(vm *exec.VirtualMachine) int64 {
				if c.enableLogging {
					frame := vm.GetCurrentFrame()
					dataPtr := int(uint32(frame.Locals[0]))
					dataLen := int(uint32(frame.Locals[1]))

					logger := log.Contracts("log")
					logger.Debug().
						Hex("contract_id", c.contractID[:]).
						Msg(string(vm.Memory[dataPtr : dataPtr+dataLen]))
				}
				return 0
			}
		case "_round_id":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				outPtr := int(uint32(frame.Locals[0]))
				outLen := int(uint32(frame.Locals[1]))

				latestRoundID := c.ctx.round.ID[:]
				if outLen != len(latestRoundID) {
					return 1
				}
				copy(vm.Memory[outPtr:outPtr+outLen], latestRoundID)
				return 0
			}
		case "_verify_ed25519":
			var gas uint64
			var ok bool
			if gas, ok = sys.GasTable["wavelet.verify.ed25519"]; !ok {
				panic("gas entry not found")
			}

			return func(vm *exec.VirtualMachine) int64 {
				vm.Gas += gas

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
		case "_hash_blake2b_512":
			var gas uint64
			var ok bool
			if gas, ok = sys.GasTable["wavelet.hash.blake2b512"]; !ok {
				panic("gas entry not found")
			}

			return func(vm *exec.VirtualMachine) int64 {
				vm.Gas += gas

				frame := vm.GetCurrentFrame()
				dataPtr, dataLen := int(uint32(frame.Locals[0])), int(uint32(frame.Locals[1]))
				outPtr, outLen := int(uint32(frame.Locals[2])), int(uint32(frame.Locals[3]))
				if outLen != blake2b.Size {
					return 1
				}

				data := vm.Memory[dataPtr : dataPtr+dataLen]
				out := vm.Memory[outPtr : outPtr+outLen]

				b := blake2b.Sum512(data)
				copy(out, b[:])
				return 0
			}
		default:
			panic("unknown field")
		}
	default:
		panic("unknown module")
	}
}

func (c *ContractExecutor) ResolveGlobal(module, field string) int64 {
	panic("no global variables")
}
