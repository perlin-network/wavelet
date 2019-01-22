package wavelet

import (
	"github.com/perlin-network/wavelet/log"

	"github.com/perlin-network/life/exec"

	"github.com/perlin-network/graph/database"
	"github.com/pkg/errors"
)

// Contract represents a smart contract on Perlin.
type Contract struct {
	TransactionID string `json:"transaction_id,omitempty"`
	Code          []byte `json:"code,omitempty"`
}

type queryableEntity interface {
	Load(key string) ([]byte, bool)
	Store(key string, value []byte)
}

type stateDeltaLogger struct {
	entity queryableEntity
	state  map[string][]byte

	disabled bool
}

func newStateDeltaLogger(entity queryableEntity, disabled bool) stateDeltaLogger {
	return stateDeltaLogger{entity, make(map[string][]byte), disabled}
}

func (s stateDeltaLogger) get(key string) []byte {
	if s.disabled {
		panic("state logging is disabled")
	}

	var val []byte
	var exists bool

	if val, exists = s.state[key]; exists {
		return val
	}

	val, _ = s.entity.Load(key)
	s.state[key] = val

	return val
}

func (s stateDeltaLogger) set(key string, val []byte) {
	if s.disabled {
		panic("state logging is disabled")
	}

	s.state[key] = val
}

type contractExecutor struct {
	contractGasPolicy
	stateDeltaLogger

	contract *Account
	sender   []byte
	payload  []byte
	pending  []*database.Transaction
}

func newContractExecutor(contract *Account, sender []byte, payload []byte, gasPolicy contractGasPolicy) contractExecutor {
	return contractExecutor{
		stateDeltaLogger:  newStateDeltaLogger(contract, false),
		contractGasPolicy: gasPolicy,

		sender:   sender,
		payload:  payload,
		contract: contract,
	}
}

func (c contractExecutor) run(code []byte, entry string) error {
	vm, err := exec.NewVirtualMachine(code, exec.VMConfig{
		DefaultMemoryPages: 1238,
		DefaultTableSize:   65536,
		GasLimit:           c.gasLimit,
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

type contractGasPolicy struct {
	gasTable map[string]int64
	gasLimit uint64
}

func (c contractGasPolicy) GetCost(name string) int64 {
	if c.gasTable == nil {
		return 1
	}

	if v, ok := c.gasTable[name]; ok {
		return v
	}

	log.Fatal().Msgf("instruction %s not found in gas table", name)

	return 1
}

func (c contractExecutor) ResolveFunc(module, field string) exec.FunctionImport {
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

				tagPtr := int(uint32(frame.Locals[0]))
				tagLen := int(uint32(frame.Locals[1]))
				payloadPtr := int(uint32(frame.Locals[2]))
				payloadLen := int(uint32(frame.Locals[3]))

				tag := string(vm.Memory[tagPtr : tagPtr+tagLen])
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

				c.stateDeltaLogger.set(string(merge(ContractCustomStatePrefix, writeBytes(key))), val)
				return 0
			}
		case "_get_len":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				keyPtr := int(uint32(frame.Locals[0]))
				keyLen := int(uint32(frame.Locals[1]))
				key := string(vm.Memory[keyPtr : keyPtr+keyLen])

				return int64(len(c.stateDeltaLogger.get(string(merge(ContractCustomStatePrefix, writeBytes(key))))))
			}
		case "_get":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				keyPtr := int(uint32(frame.Locals[0]))
				keyLen := int(uint32(frame.Locals[1]))
				outPtr := int(uint32(frame.Locals[2]))

				key := string(vm.Memory[keyPtr : keyPtr+keyLen])

				copy(vm.Memory[outPtr:], c.stateDeltaLogger.get(string(merge(ContractCustomStatePrefix, writeBytes(key)))))
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

func (c contractExecutor) ResolveGlobal(module, field string) int64 {
	panic("no global variables")
}
