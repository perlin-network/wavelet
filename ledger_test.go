package wavelet

import (
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/security"
	"os"
	"testing"
)

const databasePath = "testdb"
const servicesPath = "cmd/services"

func BenchmarkLedger(b *testing.B) {
	b.StopTimer()

	log.Disable()

	keys, err := crypto.FromPrivateKey(security.SignaturePolicy, "a6a193b4665b03e6df196ab7765b04a01de00e09c4a056f487019b5e3565522fd6edf02c950c6e091cd2450552a52febbb3d29b38c22bb89b0996225ef5ec972")
	if err != nil {
		b.Fatal(err)
	}

	ledger := NewLedger(databasePath, servicesPath)

	defer os.RemoveAll(databasePath)
	defer ledger.Graph.Cleanup()

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		parents, err := ledger.Resolver.FindEligibleParents()
		if err != nil {
			b.Fatal(err)
		}

		wired := &wire.Transaction{
			Sender:  keys.PublicKeyHex(),
			Nonce:   uint64(i),
			Parents: parents,
			Tag:     "nop",
		}

		encoded, err := wired.Marshal()
		if err != nil {
			b.Fatal(err)
		}

		wired.Signature = security.Sign(keys.PrivateKey, encoded)

		id, _, err := ledger.RespondToQuery(wired)
		if err != nil {
			b.Fatal(err)
		}

		tx, err := ledger.Store.GetBySymbol(id)
		if err != nil {
			b.Fatal(err)
		}

		err = ledger.HandleSuccessfulQuery(tx)
		if err != nil {
			b.Fatal(err)
		}

		ledger.Step()
	}
}
