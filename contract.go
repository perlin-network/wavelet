package wavelet

import (
	"github.com/perlin-network/life/compiler"
	"github.com/perlin-network/life/exec"
	"github.com/perlin-network/life/utils"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/payload"
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
	EnableLogging bool

	gasTable map[string]int64

	contractID [TransactionIDSize]byte
	ctx        *TransactionContext

	payload, result []byte

	vm *exec.VirtualMachine
}

func NewContractExecutor(contractID [TransactionIDSize]byte, ctx *TransactionContext, gasLimit uint64) (*ContractExecutor, error) {
	executor := &ContractExecutor{contractID: contractID, ctx: ctx}

	code, available := ctx.ReadAccountContractCode(contractID)
	if !available {
		return nil, errors.Wrapf(ErrNotSmartContract, "%x", contractID)
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

	vm, err := exec.NewVirtualMachine(code, config, executor, executor)
	if err != nil {
		return nil, errors.Wrap(err, "contract: failed to init smart contract vm")
	}

	executor.vm = vm

	return executor, nil
}

func (c *ContractExecutor) WithGasTable(gasTable map[string]int64) *ContractExecutor {
	c.gasTable = gasTable
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

func (c *ContractExecutor) Run(amount uint64, entry string, params ...byte) error {
	id := c.ctx.Transaction().ID
	sender := c.ctx.Transaction().Sender

	c.payload = append(payload.NewWriter(nil).
		WriteBytes(id[:]).
		WriteBytes(sender[:]).
		WriteUint64(amount).
		Bytes(), params...)

	entry = "_contract_" + entry

	entryID, exists := c.vm.GetFunctionExport(entry)
	if !exists {
		return errors.Wrapf(ErrContractFunctionNotFound, "`%s` does not exist", entry)
	}

	// Load memory snapshot if available.
	mem, err := c.loadMemorySnapshot()
	if err != nil {
		return errors.Wrap(err, "contract: failed to load memory snapshot")
	}

	if mem != nil {
		c.vm.Memory = mem
	}

	// Execute virtual machine.
	c.vm.Ignite(entryID)

	for !c.vm.Exited {
		c.vm.Execute()

		if c.vm.Delegate != nil {
			c.vm.Delegate()
			c.vm.Delegate = nil
		}
	}

	if c.vm.ExitError != nil {
		return utils.UnifyError(c.vm.ExitError)
	}

	// Save memory snapshot.
	c.saveMemorySnapshot(c.vm.Memory)

	return nil
}

func (c *ContractExecutor) GetCost(key string) int64 {
	if c.gasTable == nil {
		return 1
	}

	if cost, ok := c.gasTable[key]; ok {
		return cost
	}

	log.Fatal().Msgf("Instruction %s not found in gas table.", key)

	return 1
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
		case "_provide_result":
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
				if c.EnableLogging {
					frame := vm.GetCurrentFrame()
					dataPtr := int(uint32(frame.Locals[0]))
					dataLen := int(uint32(frame.Locals[1]))

					contractID := c.contractID

					log.Info().
						Hex("contract_id", contractID[:]).
						Msgf(string(vm.Memory[dataPtr : dataPtr+dataLen]))
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
