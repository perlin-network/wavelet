package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/life/compiler"
	"github.com/perlin-network/life/exec"
	"github.com/perlin-network/life/utils"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
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
	contractID common.AccountID
	ctx        *TransactionContext

	gasTable       map[string]uint64
	header, result []byte

	enableLogging bool
}

func NewContractExecutor(contractID common.AccountID, ctx *TransactionContext) *ContractExecutor {
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

func (c *ContractExecutor) SaveMemorySnapshot(contractID common.AccountID, memory []byte) {
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

func (c *ContractExecutor) Run(amount, gasLimit uint64, entry string, params ...byte) ([]byte, uint64, error) {
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

	c.header = make([]byte, common.SizeTransactionID+common.SizeAccountID+8)

	copy(c.header[0:common.SizeTransactionID], tx.ID[:])
	copy(c.header[common.SizeTransactionID:common.SizeTransactionID+common.SizeAccountID], tx.Sender[:])

	binary.LittleEndian.PutUint64(c.header[common.SizeTransactionID+common.SizeAccountID:8+common.SizeTransactionID+common.SizeAccountID], amount)

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

					logger := log.Contract(c.contractID, "log")
					logger.Log().Msg(string(vm.Memory[dataPtr : dataPtr+dataLen]))
				}
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
