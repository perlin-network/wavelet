package wavelet_test

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/processor"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"testing"
)

func TestNewLedger(t *testing.T) {
	genesisPath := "cmd/wavelet/config/genesis.json"

	kv := store.NewInmem()

	ledger := wavelet.NewLedger(kv, genesisPath)
	ledger.RegisterProcessor(sys.TagNop, new(processor.NopProcessor))
	ledger.RegisterProcessor(sys.TagTransfer, new(processor.TransferProcessor))
	ledger.RegisterProcessor(sys.TagStake, new(processor.StakeProcessor))
}
