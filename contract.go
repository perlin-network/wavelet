package wavelet

import (
	"github.com/perlin-network/life/compiler"
	"github.com/perlin-network/life/exec"
	"github.com/perlin-network/life/utils"
	"github.com/perlin-network/noise/payload"
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
	gasTable      map[string]uint64
	enableLogging bool

	contractID common.TransactionID
	ctx        *TransactionContext

	payload, result []byte
}

func NewContractExecutor(contractID common.TransactionID, ctx *TransactionContext) (*ContractExecutor, error) {
	executor := &ContractExecutor{contractID: contractID, ctx: ctx}

	return executor, nil
}

func (c *ContractExecutor) WithGasTable(gasTable map[string]uint64) *ContractExecutor {
	c.gasTable = gasTable
	return c
}

func (c *ContractExecutor) EnableLogging() *ContractExecutor {
	c.enableLogging = true
	return c
}

func (c *ContractExecutor) loadMemorySnapshot() ([]byte, error) {
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

func (c *ContractExecutor) saveMemorySnapshot(mem []byte) {
	numPages := uint64(len(mem) / PageSize)

	c.ctx.WriteAccountContractNumPages(c.contractID, numPages)

	for pageIdx := uint64(0); pageIdx < numPages; pageIdx++ {
		old, _ := c.ctx.ReadAccountContractPage(c.contractID, pageIdx)

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

			// If the page is empty, save an empty byte array. Else, save the pages content.
			if allZero {
				c.ctx.WriteAccountContractPage(c.contractID, pageIdx, []byte{})
			} else {
				c.ctx.WriteAccountContractPage(c.contractID, pageIdx, mem[pageIdx*PageSize:(pageIdx+1)*PageSize])
			}
		}
	}
}

func (c *ContractExecutor) Run(amount, gasLimit uint64, entry string, params ...byte) ([]byte, uint64, error) {
	id := c.ctx.Transaction().ID
	sender := c.ctx.Transaction().Sender

	code, available := c.ctx.ReadAccountContractCode(c.contractID)
	if !available {
		return nil, 0, errors.Wrapf(ErrNotSmartContract, "%x", c.contractID)
	}

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
		return nil, 0, errors.Wrap(err, "contract: failed to init smart contract vm")
	}

	c.payload = append(payload.NewWriter(nil).
		WriteBytes(id[:]).
		WriteBytes(sender[:]).
		WriteUint64(amount).
		Bytes(), params...)

	entry = "_contract_" + entry

	entryID, exists := vm.GetFunctionExport(entry)
	if !exists {
		return nil, 0, errors.Wrapf(ErrContractFunctionNotFound, "`%s` does not exist", entry)
	}

	// Load memory snapshot if available.
	mem, err := c.loadMemorySnapshot()
	if err != nil {
		return nil, 0, errors.Wrap(err, "contract: failed to load memory snapshot")
	}

	if mem != nil {
		vm.Memory = mem
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
		return nil, 0, utils.UnifyError(vm.ExitError)
	}

	// Save memory snapshot.
	c.saveMemorySnapshot(vm.Memory)

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
				return int64(len(c.payload))
			}
		case "_payload":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				outPtr := int(uint32(frame.Locals[0]))
				copy(vm.Memory[outPtr:], c.payload)
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
