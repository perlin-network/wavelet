package wavelet_test

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewLedger(t *testing.T) {
	genesisPath := "cmd/wavelet/config/genesis.json"

	kv := store.NewInmem()

	ledger := wavelet.NewLedger(kv, genesisPath)
	ledger.RegisterProcessor(sys.TagNop, wavelet.ProcessNopTransaction)
	ledger.RegisterProcessor(sys.TagTransfer, wavelet.ProcessTransferTransaction)
	ledger.RegisterProcessor(sys.TagContract, wavelet.ProcessContractTransaction)
	ledger.RegisterProcessor(sys.TagStake, wavelet.ProcessStakeTransaction)

	keys := skademlia.RandomKeys()

	tx, err := ledger.NewTransaction(keys, sys.TagNop, nil)
	assert.NoError(t, err)
	assert.NotNil(t, tx)
}
