package wavelet

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/params"

	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/life/exec"

	"github.com/pkg/errors"
)

const (
	StoreGlobal = uint32(iota)
	StoreAccount
)

const (
	InternalProcessOk = uint32(iota)
	InternalProcessErr
	InternalProcessIgnore
)

var (
	ContractPrefix            = writeBytes("C-")
	ContractCustomStatePrefix = writeBytes("CS-")
)

type service struct {
	*sync.Mutex
	*state

	name string

	globals  map[string][]byte
	accounts map[string]*Account

	// The current transaction being fed into this service.
	tx *database.Transaction

	// Pending new transactions. Usually created by smart contracts.
	pendingNewTransactions []*database.Transaction

	// The current account being modified.
	accountID string

	vm    *exec.VirtualMachine
	entry int
}

func NewService(state *state, name string) *service {
	return &service{Mutex: new(sync.Mutex), state: state, name: name}
}

func (s *service) Run(tx *database.Transaction, accounts map[string]*Account) ([]*database.Transaction, error) {
	s.Lock()
	defer s.Unlock()

	s.tx = tx
	s.pendingNewTransactions = nil

	// Reset service state for each run.
	s.globals = make(map[string][]byte)
	s.accounts = accounts
	s.accountID = ""

	start := time.Now()

	ret, err := s.vm.Run(s.entry)
	if err != nil {
		s.vm.PrintStackTrace()
		panic(errors.Errorf("Transaction processor [%s] panicked during handling tx: %+v", s.name, tx))
	}

	log.Debug().TimeDiff("duration", time.Now(), start).Msgf("Executed service %s.", s.name)

	switch uint32(ret) {
	case InternalProcessErr:
		return nil, errors.New("processor reported an error")
	case InternalProcessIgnore:
		return nil, nil
	}

	return s.pendingNewTransactions, nil
}

func (s *service) globalQuery(key string) (value []byte) {
	// Locally load global key if not loaded beforehand.
	if v, exists := s.globals[key]; exists {
		value = v
	} else {
		// TODO: Query global store.
	}

	return
}

func (s *service) accountQuery(id string, key string) []byte {
	if s.accounts[id] == nil {
		s.accounts[id] = NewAccount(s.Ledger, []byte(id))
	}

	v, _ := s.accounts[id].Load(key)
	return v
}

func (s *service) accountWrite(id string, key string, val []byte) {
	if s.accounts[id] == nil {
		s.accounts[id] = NewAccount(s.Ledger, []byte(id))
	}

	s.accounts[id].Store(key, val)
}

func (s *service) ResolveFunc(module, field string) exec.FunctionImport {
	switch module {
	case "env":
		switch field {
		case "abort":
			return func(vm *exec.VirtualMachine) int64 {
				panic(errors.New("abort called"))
			}
		case "_log":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				msgPtr := int(uint32(frame.Locals[0]))
				msgLen := int(uint32(frame.Locals[1]))
				msg := string(vm.Memory[msgPtr : msgPtr+msgLen])

				log.Debug().Msgf("[%s] %s", s.name, msg)
				return int64(InternalProcessOk)
			}
		case "_tag":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				outPtr := int(uint32(frame.Locals[0]))
				copy(vm.Memory[outPtr:], s.tx.Tag)

				return int64(InternalProcessOk)
			}
		case "_tag_len":
			return func(vm *exec.VirtualMachine) int64 {
				return int64(len(s.tx.Tag))
			}
		case "_payload":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				outPtr := int(uint32(frame.Locals[0]))
				copy(vm.Memory[outPtr:], s.tx.Payload)

				return int64(InternalProcessOk)
			}
		case "_payload_len":
			return func(vm *exec.VirtualMachine) int64 {
				return int64(len(s.tx.Payload))
			}
		case "_load_account":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()

				keyPtr := int(uint32(frame.Locals[0]))
				keyLen := int(uint32(frame.Locals[1]))
				key, err := hex.DecodeString(string(vm.Memory[keyPtr : keyPtr+keyLen]))

				if err != nil {
					return int64(InternalProcessErr)
				}

				s.accountID = string(key)

				return int64(InternalProcessOk)
			}
		case "_load_sender":
			return func(vm *exec.VirtualMachine) int64 {
				publicKey, err := hex.DecodeString(s.tx.Sender)
				if err != nil {
					log.Info().Msg("cannot decode sender")
					return int64(InternalProcessErr)
				}

				s.accountID = string(publicKey)

				return int64(InternalProcessOk)
			}
		case "_sender_id":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				outPtr := int(uint32(frame.Locals[0]))
				outLen := int(uint32(frame.Locals[1]))

				//publicKey, err := hex.DecodeString(s.tx.Sender)
				//if err != nil {
				//	return int64(InternalProcessErr)
				//}

				// TODO: have _sender_id be represented as decoded hex sender public key
				return int64(copy(vm.Memory[outPtr:outPtr+outLen], writeBytes(s.tx.Sender)))
			}
		case "_load":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				storeID := uint32(frame.Locals[0])

				keyPtr := int(uint32(frame.Locals[1]))
				keyLen := int(uint32(frame.Locals[2]))
				key := string(vm.Memory[keyPtr : keyPtr+keyLen])

				outPtr := int(uint32(frame.Locals[3]))

				switch storeID {
				case StoreGlobal:
					copy(vm.Memory[outPtr:], s.globalQuery(key))
				case StoreAccount:
					if s.accountID == "" {
						return 0
					}

					copy(vm.Memory[outPtr:], s.accountQuery(s.accountID, key))
				default:
					panic(errors.Errorf("unknown store specified: %d", storeID))
				}

				return 0
			}
		case "_load_len":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				storeID := uint32(frame.Locals[0])

				keyPtr := int(uint32(frame.Locals[1]))
				keyLen := int(uint32(frame.Locals[2]))
				key := string(vm.Memory[keyPtr : keyPtr+keyLen])

				switch storeID {
				case StoreGlobal:
					// Locally load account key if not loaded beforehand.
					value := s.globalQuery(key)
					return int64(len(value))
				case StoreAccount:
					if s.accountID == "" {
						return 0
					}

					value := s.accountQuery(s.accountID, key)
					return int64(len(value))
				default:
					panic(errors.Errorf("unknown store specified: %d", storeID))
				}

				return 0
			}
		case "_store":
			return func(vm *exec.VirtualMachine) int64 {
				storeID := uint32(vm.GetCurrentFrame().Locals[0])

				keyPtr := int(uint32(vm.GetCurrentFrame().Locals[1]))
				keyLen := int(uint32(vm.GetCurrentFrame().Locals[2]))
				key := string(vm.Memory[keyPtr : keyPtr+keyLen])

				valuePtr := int(uint32(vm.GetCurrentFrame().Locals[3]))
				valueLen := int(uint32(vm.GetCurrentFrame().Locals[4]))
				value := vm.Memory[valuePtr : valuePtr+valueLen]

				switch storeID {
				case StoreGlobal:
					copy(s.globals[key], value)
				case StoreAccount:
					if s.accountID == "" {
						return int64(InternalProcessErr)
					}

					buf := make([]byte, len(value))
					copy(buf, value)

					s.accountWrite(s.accountID, key, buf)
				default:
					panic(errors.Errorf("unknown store specified: %d", storeID))
				}

				return int64(InternalProcessOk)
			}
		case "_activate_contract":
			return func(vm *exec.VirtualMachine) int64 {
				contractIDPtr := int(uint32(vm.GetCurrentFrame().Locals[0]))
				contractIDLen := int(uint32(vm.GetCurrentFrame().Locals[1]))

				encodedContractID := string(vm.Memory[contractIDPtr : contractIDPtr+contractIDLen])

				contractID, err := hex.DecodeString(encodedContractID)
				if err != nil {
					return int64(InternalProcessOk)
				}

				if !bytes.HasPrefix(contractID, ContractPrefix) {
					return int64(InternalProcessOk) // no need to activate
				}

				senderID, err := hex.DecodeString(s.tx.Sender)
				if err != nil {
					return int64(InternalProcessOk)
				}

				if bytes.HasPrefix(senderID, ContractPrefix) {
					return int64(InternalProcessOk) // cannot activate recursively
				}

				contractCode := s.accountQuery(writeString(contractID), params.KeyContractCode)
				if contractCode == nil {
					return int64(InternalProcessOk)
				}

				var localNewTx []*database.Transaction

				executor := newContractExecutor(s.account, senderID, s.tx.Payload, contractGasPolicy{nil, 100000})

				err = executor.run(contractCode, "contract_main")
				if err != nil {
					log.Warn().Err(err).Msg("smart contract exited with error")
					return int64(InternalProcessOk)
				}

				s.pendingNewTransactions = append(s.pendingNewTransactions, localNewTx...)
				return int64(InternalProcessOk)
			}
		case "_decode_and_create_contract":
			return func(vm *exec.VirtualMachine) int64 {
				senderID, err := hex.DecodeString(s.tx.Sender)
				if err != nil {
					return int64(InternalProcessOk)
				}

				if bytes.HasPrefix(senderID, ContractPrefix) {
					return int64(InternalProcessErr) // a Contract cannot create another one
				}

				contractID := ContractID(s.tx.Id)

				// This struct is defined in
				// github.com/perlin-network/transaction-processor-rs/builtin/contract/src/lib.rs
				var payload struct {
					Code string `json:"code"`
				}

				err = json.Unmarshal(s.tx.Payload, &payload)
				if err != nil {
					return int64(InternalProcessErr)
				}

				decoded, err := base64.StdEncoding.DecodeString(payload.Code)
				if err != nil {
					return int64(InternalProcessErr)
				}

				s.accountWrite(contractID, params.KeyContractCode, decoded)

				return int64(InternalProcessOk)
			}
		case "_create_contract":
			return func(vm *exec.VirtualMachine) int64 {
				frame := vm.GetCurrentFrame()
				codePtr := int(uint32(frame.Locals[0]))
				codeLen := int(uint32(frame.Locals[1]))
				code := vm.Memory[codePtr : codePtr+codeLen]

				senderID, err := hex.DecodeString(s.tx.Sender)
				if err != nil {
					return int64(InternalProcessOk)
				}

				if bytes.HasPrefix(senderID, ContractPrefix) {
					return int64(InternalProcessErr) // a contract cannot create another one
				}

				contractID := ContractID(s.tx.Id)

				s.accountWrite(contractID, params.KeyContractCode, code)

				return int64(InternalProcessOk)
			}
		default:
			panic(errors.Errorf("unknown import resolved: %s", field))
		}
	default:
		panic(errors.Errorf("unknown module: %s", module))
	}
}

func (s *service) ResolveGlobal(module, field string) int64 {
	panic("no global variables")
}

// ContractID returns the expected ID of a smart contract given the transaction symbol which
// spawned the contract.
func ContractID(txID string) string {
	return string(merge(ContractPrefix, writeBytes(txID)))
}
