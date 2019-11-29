package wavelet

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

type SyncProvideChunk struct {
	RequestedHash []byte `json:"requested_hash"`
}

var _ log.JSONObject = (*SyncApplied)(nil)

func (s *SyncProvideChunk) MarshalEvent(ev *zerolog.Event) {
	ev.Hex("requested_hash", s.RequestedHash)
	ev.Msg("Responded to sync chunk request.")
}

func (s *SyncProvideChunk) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"requested_hash", s.RequestedHash)
}

func (s *SyncProvideChunk) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueAny(v, &s.RequestedHash, "requested_hash")
}

type SyncPullTransaction struct {
	NumTransactions int `json:"num_transactions"`
}

var _ log.JSONObject = (*SyncPullTransaction)(nil)

func (s *SyncPullTransaction) MarshalEvent(ev *zerolog.Event) {
	ev.Int("num_transactions", s.NumTransactions)
	ev.Msg("Provided transactions for a pull request.")
}

func (s *SyncPullTransaction) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"num_transactions", s.NumTransactions)
}

func (s *SyncPullTransaction) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueAny(v, &s.NumTransactions, "num_transactions")
}
