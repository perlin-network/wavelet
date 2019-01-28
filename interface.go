package wavelet

import (
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/wire"
)

// LedgerInterface provides methods expected by the ledger
type LedgerInterface interface {
	// Wavelet/state
	Snapshot() map[string]interface{}
	LoadContract(txID []byte) ([]byte, error)
	NumContracts() uint64
	PaginateContracts(offset, pageSize uint64) []*Contract
	NumTransactions() uint64
	PaginateTransactions(offset, pageSize uint64) []*database.Transaction
	ExecuteContract(txID []byte, entry string, param []byte) ([]byte, error)

	// Wavelet/rpc
	RespondToQuery(wired *wire.Transaction) ([]byte, bool, error)
	HandleSuccessfulQuery(tx *database.Transaction) error

	// Wavelet/ledger
	WasAccepted(symbol []byte) bool
	QueueForAcceptance(symbol []byte) error
	Accounts() *Accounts

	// Store
	GetBySymbol(symbol []byte) (*database.Transaction, error)
	GetChildrenBySymbol(symbol []byte) (*database.Children, error)
	GetMostRecentlyUsed(n int) [][]byte
	TransactionExists(symbol []byte) bool

	// Graph
	Cleanup() error
	FindEligibleParents() (parents [][]byte, err error)
	CountAscendants(symbol []byte, threshold int) (numReachable int)
	IsStronglyPreferred(symbol []byte) bool
}
