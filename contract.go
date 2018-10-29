package wavelet

import (
	"encoding/json"

	"github.com/perlin-network/wavelet/log"

	"github.com/perlin-network/life/exec"

	"github.com/pkg/errors"
)

// Contract represents a smart contract on Perlin.
type Contract struct {
	Code string `json:"code"`
}

// NewContract returns a new smart contract object.
func NewContract(code string) *Contract {
	contract := &Contract{
		Code: code,
	}

	return contract
}

// UnmarshalContract unmarshals json-encoded bytes into a contract object
func UnmarshalContract(bytes []byte) (*Contract, error) {
	var contract Contract
	err := json.Unmarshal(bytes, contract)
	if err != nil {
		return nil, err
	}
	return &contract, nil
}

type ContractExecutor struct {
	GasTable               map[string]int64
	GasLimit               uint64
	Code                   []byte
	GetActivationReason    func() []byte
	GetActivationReasonLen func() int
	GetDataItem            func(key string) []byte          // can panic
	GetDataItemLen         func(key string) int             // can panic
	SetDataItem            func(key string, val []byte)     // can panic
	QueueTransaction       func(tag string, payload []byte) // can panic
}

func (e *ContractExecutor) Run() error {
	vm, err := exec.NewVirtualMachine(e.Code, exec.VMConfig{
		DefaultMemoryPages:   128,
		DefaultTableSize:     65536,
		GasLimit:             e.GasLimit,
		DisableFloatingPoint: true,
	}, e, e)
	if err != nil {
		return err
	}

	entryID, ok := vm.GetFunctionExport("contract_main")
	if !ok {
		return errors.Errorf("contract_main not found")
	}
	_, err = vm.Run(entryID)
	if err != nil {
		return err
	}
	return nil
}

func (e *ContractExecutor) GetCost(name string) int64 {
	if e.GasTable == nil {
		return 1
	}

	if v, ok := e.GasTable[name]; ok {
		return v
	} else {
		log.Fatal().Msg("instruction not found in gas table")
	}

	return 1
}

func (e *ContractExecutor) ResolveFunc(module, field string) exec.FunctionImport {
	switch module {
	case "env":
		switch field {
		case "_send_transaction":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				tagPtr := int(uint32(frame.Locals[0]))
				tagLen := int(uint32(frame.Locals[1]))
				payloadPtr := int(uint32(frame.Locals[2]))
				payloadLen := int(uint32(frame.Locals[3]))

				tag := string(vm.Memory[tagPtr : tagPtr+tagLen])
				payload := vm.Memory[payloadPtr : payloadPtr+payloadLen]

				e.QueueTransaction(tag, payload)
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
				e.SetDataItem(key, val)
				return 0
			}
		case "_get_len":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				keyPtr := int(uint32(frame.Locals[0]))
				keyLen := int(uint32(frame.Locals[1]))
				key := string(vm.Memory[keyPtr : keyPtr+keyLen])
				return int64(e.GetDataItemLen(key))
			}
		case "_get":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				keyPtr := int(uint32(frame.Locals[0]))
				keyLen := int(uint32(frame.Locals[1]))
				outPtr := int(uint32(frame.Locals[2]))
				key := string(vm.Memory[keyPtr : keyPtr+keyLen])
				copy(vm.Memory[outPtr:], e.GetDataItem(key))
				return 0
			}
		case "_reason_len":
			return func(vm *exec.VirtualMachine) int64 {
				return int64(e.GetActivationReasonLen())
			}
		case "_reason":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				outPtr := int(uint32(frame.Locals[0]))
				copy(vm.Memory[outPtr:], e.GetActivationReason())
				return 0
			}
		default:
			panic("unknown field")
		}
	default:
		panic("unknown module")
	}
}

func (e *ContractExecutor) ResolveGlobal(module, field string) int64 {
	panic("no global variables")
}
