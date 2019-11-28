package wavelet

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

/*
	Sync events
*/

// event: *, level: error
type SyncError struct {
	Message string      `json:"msg"`
	Event   log.MiniLog `json:"event"`
}

var _ log.JSONObject = (*SyncOutOfSync)(nil)

func (s *SyncError) MarshalEvent(ev *zerolog.Event) {
	s.Event.MarshalEvent(ev)
	ev.Msg(s.Message)
}

func (s *SyncError) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"msg", s.Message,
		"event", s.Event)
}

func (s *SyncError) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueBatch(v,
		"msg", &s.Message,
		"event", &s.Event)
}

// event: out_of_sync
type SyncOutOfSync struct {
	CurrentBlockIndex uint64 `json:"current_block_index"`
}

var _ log.JSONObject = (*SyncOutOfSync)(nil)

func (s *SyncOutOfSync) MarshalEvent(ev *zerolog.Event) {
	ev.Uint64("current_block_index", s.CurrentBlockIndex)
	ev.Msg("Noticed that we are out of sync; downloading latest state Snapshot from our peer(s).")
}

func (s *SyncOutOfSync) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"current_block_index", s.CurrentBlockIndex)
}

func (s *SyncOutOfSync) UnmarshalValue(v *fastjson.Value) error {
	s.CurrentBlockIndex = v.GetUint64("current_block_index")
	return nil
}

// event: applying
type SyncApplying struct {
	NumChunks   int    `json:"num_chunks"`
	TargetBlock uint64 `json:"target_block"`
}

var _ log.JSONObject = (*SyncApplying)(nil)

func (s *SyncApplying) MarshalEvent(ev *zerolog.Event) {
	ev.Int("num_chunks", s.NumChunks)
	ev.Uint64("target_block", s.TargetBlock)
	ev.Msg("All chunks have been successfully verified and re-assembled into a diff. Applying diff...")
}

func (s *SyncApplying) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"num_chunks", s.NumChunks,
		"target_block", s.TargetBlock)
}

func (s *SyncApplying) UnmarshalValue(v *fastjson.Value) error {
	s.NumChunks = v.GetInt("num_chunks")
	s.TargetBlock = v.GetUint64("target_block")
	return nil
}

// event: applied
type SyncApplied struct {
	NumChunks int `json:"num_chunks"`

	OldBlockIndex uint64 `json:"old_block_index"`
	NewBlockIndex uint64 `json:"new_block_index"`

	OldBlockID BlockID `json:"old_block_id"`
	NewBlockID BlockID `json:"new_block_id"`

	NewMerkleRoot MerkleNodeID `json:"new_merkle_root"`
	OldMerkleRoot MerkleNodeID `json:"old_merkle_root"`
}

var _ log.JSONObject = (*SyncApplied)(nil)

func (s *SyncApplied) MarshalEvent(ev *zerolog.Event) {
	ev.Int("num_chunks", s.NumChunks)
	ev.Uint64("old_block_index", s.OldBlockIndex)
	ev.Uint64("new_block_index", s.NewBlockIndex)
	ev.Hex("new_block_id", s.NewBlockID[:])
	ev.Hex("old_block_id", s.OldBlockID[:])
	ev.Hex("new_merkle_root", s.NewMerkleRoot[:])
	ev.Hex("old_merkle_root", s.OldMerkleRoot[:])
	ev.Msg("Successfully built a new state snapshot out of chunk(s) we have received from peers.")
}

func (s *SyncApplied) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"num_chunks", s.NumChunks,
		"old_block_index", s.OldBlockIndex,
		"new_block_index", s.NewBlockIndex,
		"new_block_id", s.NewBlockID,
		"old_block_id", s.OldBlockID,
		"new_merkle_root", s.NewMerkleRoot,
		"old_merkle_root", s.OldMerkleRoot)
}

func (s *SyncApplied) UnmarshalValue(v *fastjson.Value) error {
	s.NumChunks = v.GetInt("num_chunks")
	s.OldBlockIndex = v.GetUint64("old_block_index")
	s.NewBlockIndex = v.GetUint64("new_block_index")

	log.ValueHex(v, s.NewBlockID, "new_block_id")
	log.ValueHex(v, s.OldBlockID, "old_block_id")

	log.ValueHex(v, s.NewMerkleRoot, "new_merkle_root")
	log.ValueHex(v, s.OldMerkleRoot, "old_merkle_root")

	return nil
}

/*
	Consensus events
*/

// event: finalized
type ConsensusFinalized struct {
	AppliedTxs  int `json:"num_applied_tx"`
	RejectedTxs int `json:"num_rejected_tx"`
	PrunedTxs   int `json:"num_pruned_tx"`

	OldBlockHeight uint64 `json:"old_block_height"`
	NewBlockHeight uint64 `json:"new_block_height"`

	OldBlockID BlockID `json:"old_block_id"`
	NewBlockID BlockID `json:"new_block_id"`
}

var _ log.JSONObject = (*ConsensusFinalized)(nil)

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

func (c *ConsensusFinalized) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"num_applied_tx", c.AppliedTxs,
		"num_rejected_tx", c.RejectedTxs,
		"num_pruned_tx", c.PrunedTxs,
		"old_block_height", c.OldBlockHeight,
		"new_block_height", c.NewBlockHeight,
		"old_block_id", c.OldBlockID,
		"new_block_id", c.NewBlockID)
}

func (c *ConsensusFinalized) UnmarshalValue(v *fastjson.Value) error {
	c.AppliedTxs = v.GetInt("num_applied_tx")
	c.RejectedTxs = v.GetInt("num_rejected_tx")
	c.PrunedTxs = v.GetInt("num_pruned_tx")
	c.OldBlockHeight = v.GetUint64("old_block_height")
	c.NewBlockHeight = v.GetUint64("new_block_height")

	log.ValueHex(v, c.OldBlockID, "old_block_id")
	log.ValueHex(v, c.NewBlockID, "new_block_id")

	return nil
}
