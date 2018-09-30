package wavelet

import (
	"encoding/hex"
	"github.com/perlin-network/wavelet/log"
)

// public key -> (key, value)
var genesis = map[string]map[string]interface{}{
	"71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858": {
		"balance": uint64(10),
	},
}

// Spawn the genesis.
func BIGBANG(ledger *Ledger) {
	for encoded, values := range genesis {
		id, err := hex.DecodeString(encoded)
		if err != nil {
			log.Fatal().Err(err).Str("public_key", encoded).Msg("Failed to decode genesis account ID.")
		}

		account := NewAccount(id)
		for key, v := range values {
			switch value := v.(type) {
			case uint64:
				account.Store(key, writeUint64(value))
			}
		}

		err = ledger.SaveAccount(account, nil)
		if err != nil {
			log.Fatal().Err(err).Str("public_key", encoded).Msg("Failed to save genesis account information.")
		}
	}
}
