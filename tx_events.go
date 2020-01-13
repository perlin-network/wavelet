package wavelet

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

/*
	TX events
*/

// event: applied
type TxApplied struct {
	Transaction
}

var _ log.Loggable = (*TxApplied)(nil)

type TxRejected struct {
	Transaction
}

var _ log.Loggable = (*TxRejected)(nil)

/*
	TX Applier events
*/

type ContractGas struct {
	SenderID   AccountID `json:"sender_id"`
	ContractID [32]byte  `json:"contract_id"`
	Gas        uint64    `json:"gas"`
	GasLimit   uint64    `json:"gas_limit"`
}

func (c *ContractGas) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("sender_id", c.SenderID[:])
	ev.Hex("contract_id", c.ContractID[:])
	ev.Uint64("gas", c.Gas)
	ev.Uint64("gas_limit", c.GasLimit)
	ev.Msg("Exceeded gas limit while invoking smart contract function.")
}

func (c *ContractGas) UnmarshalValue(value *fastjson.Value) error {
	return log.ValueBatch(value,
		"sender_id", c.SenderID[:],
		"contract_id", c.ContractID[:],
		"gas", &c.Gas,
		"gas_limit", &c.GasLimit)
}
