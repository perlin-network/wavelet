package wavelet

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	accounts map[string]map[string][]byte

	// The current transaction being fed into this service.
	tx *database.Transaction

	// Pending new transactions. Usually created by smart contracts.
	pendingNewTransactions []*database.Transaction

	// The current account being modified.
	account *Account

	vm    *exec.VirtualMachine
	entry int
}

func NewService(state *state, name string) *service {
	return &service{Mutex: new(sync.Mutex), state: state, name: name}
}

func (s *service) Run(tx *database.Transaction) ([]*Delta, []*database.Transaction, error) {
	s.Lock()
	defer s.Unlock()

	s.tx = tx
	s.pendingNewTransactions = nil

	// Reset service state for each run.
	s.globals = make(map[string][]byte)
	s.accounts = make(map[string]map[string][]byte)
	s.account = nil

	start := time.Now()

	ret, err := s.vm.Run(s.entry)
	if err != nil {
		s.vm.PrintStackTrace()
		panic(errors.Errorf("Transaction processor [%s] panicked during handling tx: %+v", s.name, tx))
	}

	log.Debug().TimeDiff("duration", time.Now(), start).Msgf("Executed service %s.", s.name)

	switch uint32(ret) {
	case InternalProcessErr:
		return nil, nil, errors.New("processor reported an error")
	case InternalProcessIgnore:
		return nil, nil, nil
	}

	var changes []*Delta

	// Append global store changes. Omit account to denote a change in the global store.
	for key, val := range s.globals {
		changes = append(changes, &Delta{
			Key:      key,
			NewValue: val,
		})
	}

	// Append account store changes.
	for account, store := range s.accounts {
		for key, val := range store {
			changes = append(changes, &Delta{
				Account:  writeBytes(account),
				Key:      key,
				NewValue: val,
			})
		}
	}

	return changes, s.pendingNewTransactions, nil
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

func (s *service) accountQuery(id string, key string) (value []byte) {
	if s.accounts[id] == nil {
		s.accounts[id] = make(map[string][]byte)
	}

	// Locally load account key if not loaded beforehand.
	if v, exists := s.accounts[id][key]; exists {
		value = v
	} else {
		_, value = s.account.State.Load(key)
		s.accounts[id][key] = value
	}

	return
}

func (s *service) ResolveFunc(module, field string) exec.FunctionImport {
	switch module {
	case "env":
		switch field {
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

				account, err := s.LoadAccount(key)

				if err != nil {
					account = NewAccount(key)
				}

				s.account = account

				return int64(InternalProcessOk)
			}
		case "_load_sender":
			return func(vm *exec.VirtualMachine) int64 {
				publicKey, err := hex.DecodeString(s.tx.Sender)
				if err != nil {
					return int64(InternalProcessErr)
				}

				account, err := s.LoadAccount(publicKey)
				if err != nil {
					return int64(InternalProcessErr)
				}

				s.account = account

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
					if s.account == nil {
						return 0
					}

					copy(vm.Memory[outPtr:], s.accountQuery(string(s.account.PublicKey), key))
				default:
					panic(fmt.Errorf("unknown store specified: %d", storeID))
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
					if s.account == nil {
						return 0
					}

					value := s.accountQuery(string(s.account.PublicKey), key)
					return int64(len(value))
				default:
					panic(fmt.Errorf("unknown store specified: %d", storeID))
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
					if s.account == nil {
						return int64(InternalProcessErr)
					}

					buf := make([]byte, len(value))
					copy(buf, value)

					s.accounts[string(s.account.PublicKey)][key] = buf
				default:
					panic(fmt.Errorf("unknown store specified: %d", storeID))
				}

				return int64(InternalProcessOk)
			}
		case "_activate_contract":
			return func(vm *exec.VirtualMachine) int64 {
				contractIDPtr := int(uint32(vm.GetCurrentFrame().Locals[0]))
				contractIDLen := int(uint32(vm.GetCurrentFrame().Locals[1]))
				reasonPtr := int(uint32(vm.GetCurrentFrame().Locals[2]))
				reasonLen := int(uint32(vm.GetCurrentFrame().Locals[3]))

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

				account, err := s.state.LoadAccount(contractID)
				if err != nil {
					return int64(InternalProcessOk) // always report ok.
				}

				contractCode, ok := account.Load(params.KeyContractCode)
				if !ok {
					return int64(InternalProcessOk)
				}

				reason := vm.Memory[reasonPtr : reasonPtr+reasonLen]

				var localNewTx []*database.Transaction

				executor := &ContractExecutor{
					GasTable: nil,
					GasLimit: 100000, // TODO: Set the limit properly
					Code:     contractCode,
					QueueTransaction: func(tag string, payload []byte) {
						tx := &database.Transaction{
							Sender:  encodedContractID,
							Tag:     tag,
							Payload: payload,
						}
						localNewTx = append(localNewTx, tx)
					},
					GetActivationReasonLen: func() int { return len(reason) },
					GetActivationReason:    func() []byte { return reason },
					GetDataItemLen: func(key string) int {
						val, _ := s.account.Load(string(merge(ContractCustomStatePrefix, writeBytes(key))))
						return len(val)
					},
					GetDataItem: func(key string) []byte {
						val, _ := s.account.Load(string(merge(ContractCustomStatePrefix, writeBytes(key))))
						return val
					},
					SetDataItem: func(key string, val []byte) {
						s.account.Store(string(merge(ContractCustomStatePrefix, writeBytes(key))), val)
					},
				}

				err = executor.Run()
				if err != nil {
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
					return int64(InternalProcessErr) // a contract cannot create another one
				}

				contractID := asContractID(s.tx.Id)

				if s.accounts[contractID] == nil {
					s.accounts[contractID] = make(map[string][]byte)
				}

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

				s.accounts[contractID][params.KeyContractCode] = decoded

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

				contractID := asContractID(s.tx.Id)

				if s.accounts[contractID] == nil {
					s.accounts[contractID] = make(map[string][]byte)
				}

				s.accounts[contractID][params.KeyContractCode] = code

				return int64(InternalProcessOk)
			}
		default:
			panic(fmt.Errorf("unknown import resolved: %s", field))
		}
	default:
		panic(fmt.Errorf("unknown module: %s", module))
	}
}

func (s *service) ResolveGlobal(module, field string) int64 {
	panic("no global variables")
}

// ContractID returns the expected ID of a smart contract given the transaction symbol which
// spawned the contract.
func asContractID(txID string) string {
	return string(merge(ContractPrefix, writeBytes(txID)))
}
