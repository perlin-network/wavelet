package wavelet

var (
	keyAccounts       = [...]byte{0x1}
	keyAccountBalance = [...]byte{0x2}
	keyAccountStake   = [...]byte{0x3}

	keyAccountContractCode     = [...]byte{0x4}
	keyAccountContractNumPages = [...]byte{0x5}
	keyAccountContractPages    = [...]byte{0x6}

	keyLedgerDifficulty = [...]byte{0x7}
	keyLedgerGenesis    = [...]byte{0x8}

	keyGraphRoot = [...]byte{0x9}
)
