package wavelet

import "golang.org/x/crypto/blake2b"

var (
	keyAccounts       = [...]byte{0x1}
	keyAccountBalance = [...]byte{0x2}
	keyAccountStake   = [...]byte{0x3}

	keyAccountContractCode     = [...]byte{0x4}
	keyAccountContractNumPages = [...]byte{0x5}
	keyAccountContractPages    = [...]byte{0x6}

	keyLedgerDifficulty = [...]byte{0x7}
	keyLedgerViewID     = [...]byte{0x8}
	keyLedgerGenesis    = [...]byte{0x9}

	keyGraphRoot = [...]byte{0x10}
)

func hashOfParentIDs(tx *Transaction) [blake2b.Size256]byte {
	var buf []byte

	for _, id := range tx.ParentIDs {
		buf = append(buf, id[:]...)
	}

	return blake2b.Sum256(buf)
}
