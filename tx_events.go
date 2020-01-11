package wavelet

import (
	"time"

	"github.com/rs/zerolog"
)

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

type ContractGas struct {
	SenderID   AccountID `json:"sender_id"`
	ContractID [32]byte  `json:"contract_id"`
	Gas        uint64    `json:"gas"`
	GasLimit   uint64    `json:"gas_limit"`
	Time       time.Time `json:"time"`
	Message    string    `json:"message"`
}

func (c *ContractGas) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("sender_id", c.SenderID[:])
	ev.Hex("contract_id", c.ContractID[:])
	ev.Uint64("gas", c.Gas)
	ev.Uint64("gas_limit", c.GasLimit)
	ev.Msg("Exceeded gas limit while invoking smart contract function.")
}
