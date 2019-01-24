package wavelet

import (
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/wire"
)

type MockLedger struct {
	SnapshotCB              func() map[string]interface{}
	LoadContractCB          func(txID string) ([]byte, error)
	NumContractsCB          func() uint64
	PaginateContractsCB     func(offset, pageSize uint64) []*Contract
	NumTransactionsCB       func() uint64
	PaginateTransactionsCB  func(offset, pageSize uint64) []*database.Transaction
	ExecuteContractCB       func(txID string, entry string, param []byte) ([]byte, error)
	RespondToQueryCB        func(wired *wire.Transaction) (string, bool, error)
	HandleSuccessfulQueryCB func(tx *database.Transaction) error
	WasAcceptedCB           func(symbol string) bool
	QueueForAcceptanceCB    func(symbol string) error
	GetBySymbolCB           func(symbol string) (*database.Transaction, error)
	GetChildrenBySymbolCB   func(symbol string) (*database.Children, error)
	GetMostRecentlyUsedCB   func(n int) []string
	TransactionExistsCB     func(symbol string) bool
	CleanupCB               func() error
	FindEligibleParentsCB   func() (parents []string, err error)
	CountAscendantsCB       func(symbol string, threshold int) (numReachable int)
	IsStronglyPreferredCB   func(symbol string) bool
}

func (m *MockLedger) Snapshot() map[string]interface{} {
	return m.SnapshotCB()
}

func (m *MockLedger) LoadContract(txID string) ([]byte, error) {
	return m.LoadContractCB(txID)
}

func (m *MockLedger) NumContracts() uint64 {
	return m.NumContractsCB()
}

func (m *MockLedger) PaginateContracts(offset, pageSize uint64) []*Contract {
	return m.PaginateContractsCB(offset, pageSize)
}

func (m *MockLedger) NumTransactions() uint64 {
	return m.NumTransactionsCB()
}

func (m *MockLedger) PaginateTransactions(offset, pageSize uint64) []*database.Transaction {
	return m.PaginateTransactionsCB(offset, pageSize)
}

func (m *MockLedger) ExecuteContract(txID string, entry string, param []byte) ([]byte, error) {
	return m.ExecuteContractCB(txID, entry, param)
}

func (m *MockLedger) RespondToQuery(wired *wire.Transaction) (string, bool, error) {
	return m.RespondToQueryCB(wired)
}

func (m *MockLedger) HandleSuccessfulQuery(tx *database.Transaction) error {
	return m.HandleSuccessfulQueryCB(tx)
}

func (m *MockLedger) WasAccepted(symbol string) bool {
	return m.WasAcceptedCB(symbol)
}

func (m *MockLedger) QueueForAcceptance(symbol string) error {
	return m.QueueForAcceptanceCB(symbol)
}

func (m *MockLedger) GetBySymbol(symbol string) (*database.Transaction, error) {
	return m.GetBySymbolCB(symbol)
}

func (m *MockLedger) GetChildrenBySymbol(symbol string) (*database.Children, error) {
	return m.GetChildrenBySymbolCB(symbol)
}

func (m *MockLedger) GetMostRecentlyUsed(n int) []string {
	return m.GetMostRecentlyUsedCB(n)
}

func (m *MockLedger) TransactionExists(symbol string) bool {
	return m.TransactionExistsCB(symbol)
}

func (m *MockLedger) Cleanup() error {
	return m.CleanupCB()
}

func (m *MockLedger) FindEligibleParents() (parents []string, err error) {
	return m.FindEligibleParentsCB()
}

func (m *MockLedger) CountAscendants(symbol string, threshold int) (numReachable int) {
	return m.CountAscendantsCB(symbol, threshold)
}

func (m *MockLedger) IsStronglyPreferred(symbol string) bool {
	return m.IsStronglyPreferredCB(symbol)
}
