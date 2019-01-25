package wavelet

import (
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/wire"
)

// LedgerInterface provides methods expected by the ledger
type LedgerInterface interface {
	// Wavelet/state
	Snapshot() map[string]interface{}
	LoadContract(txID string) ([]byte, error)
	NumContracts() uint64
	PaginateContracts(offset, pageSize uint64) []*Contract
	NumTransactions() uint64
	PaginateTransactions(offset, pageSize uint64) []*database.Transaction
	ExecuteContract(txID string, entry string, param []byte) ([]byte, error)

	// Wavelet/rpc
	RespondToQuery(wired *wire.Transaction) (string, bool, error)
	HandleSuccessfulQuery(tx *database.Transaction) error

	// Wavelet/ledger
	WasAccepted(symbol string) bool
	QueueForAcceptance(symbol string) error
	Accounts() *Accounts

	// Store
	GetBySymbol(symbol string) (*database.Transaction, error)
	GetChildrenBySymbol(symbol string) (*database.Children, error)
	GetMostRecentlyUsed(n int) []string
	TransactionExists(symbol string) bool

	// Graph
	Cleanup() error
	FindEligibleParents() (parents []string, err error)
	CountAscendants(symbol string, threshold int) (numReachable int)
	IsStronglyPreferred(symbol string) bool
}
