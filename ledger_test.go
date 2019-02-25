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
	ledger.RegisterProcessor(sys.TagNop, new(wavelet.NopProcessor))
	ledger.RegisterProcessor(sys.TagTransfer, new(wavelet.TransferProcessor))
	ledger.RegisterProcessor(sys.TagStake, new(wavelet.StakeProcessor))

	keys := skademlia.RandomKeys()

	tx, err := ledger.NewTransaction(keys, sys.TagNop, nil)
	assert.NoError(t, err)
	assert.NotNil(t, tx)
}
