package wavelet

import (
	"github.com/perlin-network/wavelet/log"

	"github.com/perlin-network/life/exec"

	"encoding/hex"
	"fmt"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/life/utils"
	"github.com/perlin-network/wavelet/params"
	"github.com/pkg/errors"
)

var (
	ContractCustomStatePrefix = writeBytes("CS-")
	KeyContractPageNum        = ".CP"
	ContractPagePrefix        = "CP-"
)

const PageSize = 65536

// Contract represents a smart contract on Perlin.
type Contract struct {
	TransactionID string `json:"transaction_id,omitempty"`
	Code          []byte `json:"code,omitempty"`
}

type ContractExecutor struct {
	ContractGasPolicy

	contract *Account
	sender   []byte
	header   []byte
	payload  []byte
	pending  []*database.Transaction

	EnableLogging bool
	Result        []byte
}

func NewContractExecutor(contract *Account, sender []byte, header []byte, gasPolicy ContractGasPolicy) *ContractExecutor {
	return &ContractExecutor{
		ContractGasPolicy: gasPolicy,

		sender:   sender,
		header:   header,
		contract: contract,
	}
}

func (c *ContractExecutor) getMemorySnapshot() ([]byte, error) {
	contractPageNum := int(c.contract.GetUint64Field(KeyContractPageNum))
	if contractPageNum == 0 {
		return nil, nil
	}

	mem := make([]byte, PageSize*contractPageNum)
	for i := 0; i < contractPageNum; i++ {
		page, _ := c.contract.Load(fmt.Sprintf("%s%d", ContractPagePrefix, i))
		if len(page) > 0 { // zero-filled by default
			if len(page) != PageSize {
				return nil, errors.Errorf("page %d has a size of %d != PageSize", i, len(page))
			}
			copy(mem[PageSize*i:PageSize*(i+1)], page)
		}
	}

	return mem, nil
}

func (c *ContractExecutor) setMemorySnapshot(mem []byte) {
	newPageNum := len(mem) / PageSize
	c.contract.SetUint64Field(KeyContractPageNum, uint64(newPageNum))

	for i := 0; i < newPageNum; i++ {
		pageKey := fmt.Sprintf("%s%d", ContractPagePrefix, i)
		oldContent, _ := c.contract.Load(pageKey)

		identical := true

		for j := 0; j < PageSize; j++ {
			if len(oldContent) == 0 && mem[i*PageSize+j] != 0 {
				identical = false
				break
			}
			if len(oldContent) != 0 && mem[i*PageSize+j] != oldContent[j] {
				identical = false
				break
			}
		}

		if !identical {
			allZero := true
			for j := 0; j < PageSize; j++ {
				if mem[i*PageSize+j] != 0 {
					allZero = false
					break
				}
			}
			if allZero {
				c.contract.Store(pageKey, []byte{})
			} else {
				c.contract.Store(pageKey, mem[i*PageSize:(i+1)*PageSize])
			}
		}
	}
}

func (c *ContractExecutor) newVirtualMachine() (*exec.VirtualMachine, error) {
	code, _ := c.contract.Load(params.KeyContractCode)
	return exec.NewVirtualMachine(code, exec.VMConfig{
		DefaultMemoryPages: 16,
		MaxMemoryPages:     32,

		DefaultTableSize: PageSize,
		MaxTableSize:     PageSize,

		MaxValueSlots:     4096,
		MaxCallStackDepth: 256,
		GasLimit:          c.GasLimit,
	}, c, c)
}

func (c *ContractExecutor) runVirtualMachine(vm *exec.VirtualMachine) error {
	for !vm.Exited {
		vm.Execute()
		if vm.Delegate != nil {
			vm.Delegate()
			vm.Delegate = nil
		}
	}

	if vm.ExitError != nil {
		return utils.UnifyError(vm.ExitError)
	}

	c.setMemorySnapshot(vm.Memory)
	return nil
}

func (c *ContractExecutor) Run(entry string, params ...byte) error {
	c.payload = append(c.header, params...)

	vm, err := c.newVirtualMachine()
	if err != nil {
		return err
	}

	mem, err := c.getMemorySnapshot()
	if err != nil {
		return err
	}

	if mem != nil {
		vm.Memory = mem
	}

	entry = "_contract_" + entry
	entryID, exists := vm.GetFunctionExport(entry)
	if !exists {
		return errors.Errorf("entry point `%s` not found", entry)
	}

	vm.Ignite(entryID)
	return c.runVirtualMachine(vm)
}

type ContractGasPolicy struct {
	GasTable map[string]int64
	GasLimit uint64
}

func (c *ContractGasPolicy) GetCost(name string) int64 {
	if c.GasTable == nil {
		return 1
	}

	if v, ok := c.GasTable[name]; ok {
		return v
	}

	log.Fatal().Msgf("instruction %s not found in gas table", name)

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

				tag := uint32(frame.Locals[0]) & 0xff
				payloadPtr := int(uint32(frame.Locals[1]))
				payloadLen := int(uint32(frame.Locals[2]))

				payload := vm.Memory[payloadPtr : payloadPtr+payloadLen]

				c.pending = append(c.pending, &database.Transaction{
					Sender:  c.contract.PublicKeyHex(),
					Tag:     tag,
					Payload: payload,
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
				slice := vm.Memory[dataPtr : dataPtr+dataLen]
				c.Result = make([]byte, dataLen)
				copy(c.Result, slice)
				return 0
			}
		case "_log":
			return func(vm *exec.VirtualMachine) int64 {
				if c.EnableLogging {
					frame := vm.GetCurrentFrame()
					dataPtr := int(uint32(frame.Locals[0]))
					dataLen := int(uint32(frame.Locals[1]))
					log.Info().
						Str("contract", c.contract.PublicKeyHex()).
						Bytes("content", vm.Memory[dataPtr:dataPtr+dataLen]).
						Msg("contract log")
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

// ContractID returns the expected ID of a smart contract given the transaction symbol which
// spawned the contract.
func ContractID(txID string) []byte {
	x, err := hex.DecodeString(txID)

	if err != nil {
		panic(err)
	}

	return x
}
