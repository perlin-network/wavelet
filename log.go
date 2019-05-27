package wavelet

import (
	"encoding/base64"
	"encoding/hex"
	"github.com/perlin-network/wavelet/log"
)

func logEventTX(event string, tx *Transaction, other ...interface{}) {
	var parents []string

	for _, parentID := range tx.ParentIDs {
		parents = append(parents, hex.EncodeToString(parentID[:]))
	}

	logger := log.TX(event)
	log := logger.Log().
		Hex("tx_id", tx.ID[:]).
		Hex("sender_id", tx.Sender[:]).
		Hex("creator_id", tx.Creator[:]).
		Uint64("nonce", tx.Nonce).
		Uint64("depth", tx.Depth).
		Strs("parents", parents).
		Uint8("tag", tx.Tag).
		Str("payload", base64.StdEncoding.EncodeToString(tx.Payload))

	for _, o := range other {
		switch o := o.(type) {
		case error:
			log = log.Err(o)
		}
	}

	log.Msg("")
}
