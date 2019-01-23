package wavelet

import (
	"github.com/perlin-network/wavelet/log"

	"github.com/perlin-network/life/exec"

	"github.com/perlin-network/graph/database"
	"github.com/pkg/errors"
)

var (
	ContractPrefix            = writeBytes("C-")
	ContractCustomStatePrefix = writeBytes("CS-")
)

// Contract represents a smart contract on Perlin.
type Contract struct {
	TransactionID string `json:"transaction_id,omitempty"`
	Code          []byte `json:"code,omitempty"`
}

type ContractExecutor struct {
	ContractGasPolicy

	contract *Account
	sender   []byte
	payload  []byte
	pending  []*database.Transaction
}

func NewContractExecutor(contract *Account, sender []byte, payload []byte, gasPolicy ContractGasPolicy) *ContractExecutor {
	return &ContractExecutor{
		ContractGasPolicy: gasPolicy,

		sender:   sender,
		payload:  payload,
		contract: contract,
	}
}

func (c *ContractExecutor) Run(code []byte, entry string) error {
	vm, err := exec.NewVirtualMachine(code, exec.VMConfig{
		DefaultMemoryPages: 1238,
		DefaultTableSize:   65536,
		GasLimit:           c.GasLimit,
	}, c, c)

	if err != nil {
		return err
	}

	entryID, exists := vm.GetFunctionExport(entry)
	if !exists {
		return errors.Errorf("entry point `%s` not found", entry)
	}

	_, err = vm.Run(entryID)
	if err != nil {
		return err
	}

	return nil
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

				tag := uint32(frame.Locals[0])
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
		case "_set":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				keyPtr := int(uint32(frame.Locals[0]))
				keyLen := int(uint32(frame.Locals[1]))
				valPtr := int(uint32(frame.Locals[2]))
				valLen := int(uint32(frame.Locals[3]))

				key := string(vm.Memory[keyPtr : keyPtr+keyLen])
				val := vm.Memory[valPtr : valPtr+valLen]

				valCopy := make([]byte, len(val))
				copy(valCopy, val)

				c.contract.Store(string(merge(ContractCustomStatePrefix, writeBytes(key))), valCopy)
				return 0
			}
		case "_get_len":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				keyPtr := int(uint32(frame.Locals[0]))
				keyLen := int(uint32(frame.Locals[1]))
				key := string(vm.Memory[keyPtr : keyPtr+keyLen])

				data, _ := c.contract.Load(writeString(merge(ContractCustomStatePrefix, writeBytes(key))))
				return int64(len(data))
			}
		case "_get":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				keyPtr := int(uint32(frame.Locals[0]))
				keyLen := int(uint32(frame.Locals[1]))
				outPtr := int(uint32(frame.Locals[2]))

				key := string(vm.Memory[keyPtr : keyPtr+keyLen])

				data, _ := c.contract.Load(writeString(merge(ContractCustomStatePrefix, writeBytes(key))))
				copy(vm.Memory[outPtr:], data)

				return 0
			}
		case "_sender_id_len":
			return func(vm *exec.VirtualMachine) int64 {
				return int64(len(c.sender))
			}
		case "_sender_id":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				outPtr := int(uint32(frame.Locals[0]))
				copy(vm.Memory[outPtr:], c.sender)
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
func ContractID(txID string) string {
	return string(merge(ContractPrefix, writeBytes(txID)))
}
