package log

import (
	"github.com/perlin-network/wavelet"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

// event: finalized
type ConsensusFinalized struct {
	AppliedTxs  int `json:"num_applied_tx"`
	RejectedTxs int `json:"num_rejected_tx"`
	PrunedTxs   int `json:"num_pruned_tx"`

	OldBlockHeight uint64 `json:"old_block_height"`
	NewBlockHeight uint64 `json:"new_block_height"`

	OldBlockID wavelet.BlockID `json:"old_block_id"`
	NewBlockID wavelet.BlockID `json:"new_block_id"`
}

var _ Loggable = (*ConsensusFinalized)(nil)

func (c *ConsensusFinalized) MarshalEvent(ev *zerolog.Event) {
	ev.Int("num_applied_tx", c.AppliedTxs)
	ev.Int("num_rejected_tx", c.RejectedTxs)
	ev.Int("num_pruned_tx", c.PrunedTxs)
	ev.Uint64("old_block_height", c.OldBlockHeight)
	ev.Uint64("new_block_height", c.NewBlockHeight)
	ev.Hex("old_block_id", c.OldBlockID[:])
	ev.Hex("new_block_id", c.NewBlockID[:])
	ev.Msg("Finalized block.")
}

func (c *ConsensusFinalized) UnmarshalValue(v *fastjson.Value) error {
	c.AppliedTxs = v.GetInt("num_applied_tx")
	c.RejectedTxs = v.GetInt("num_rejected_tx")
	c.PrunedTxs = v.GetInt("num_pruned_tx")
	c.OldBlockHeight = v.GetUint64("old_block_height")
	c.NewBlockHeight = v.GetUint64("new_block_height")

	ValueHex(v, c.OldBlockID, "old_block_id")
	ValueHex(v, c.NewBlockID, "new_block_id")

	return nil
}
