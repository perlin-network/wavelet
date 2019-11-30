package wavelet

import "github.com/rs/zerolog"

/*
	TX events
*/

// event: applied
type TxApplied struct {
	*Transaction
}

func (tx *TxApplied) MarshalEvent(ev *zerolog.Event) {
	if tx.Error != nil {
		ev.Err(tx.Error)
	}

	ev.Hex("id", tx.ID[:])
	ev.Hex("sender", tx.Sender[:])
	ev.Uint64("nonce", tx.Nonce)
	ev.Uint64("block", tx.Block)
	ev.Uint8("tag", uint8(tx.Tag))
	ev.Msg("Transaction applied")
}

type TxRejected struct {
	*Transaction
}

func (tx *TxRejected) MarshalEvent(ev *zerolog.Event) {
	if tx.Error != nil {
		ev.Err(tx.Error)
	}

	ev.Hex("id", tx.ID[:])
	ev.Hex("sender", tx.Sender[:])
	ev.Uint64("nonce", tx.Nonce)
	ev.Uint64("block", tx.Block)
	ev.Uint8("tag", uint8(tx.Tag))
	ev.Msg("Transaction rejected")
}
