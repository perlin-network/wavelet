package log

import (
	"encoding/hex"
	"encoding/json"

	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

type MiniLog map[string]interface{}

var _ JSONObject = (MiniLog)(nil)

func NewMiniLog() MiniLog {
	return map[string]interface{}{}
}

func (m MiniLog) Hex(k string, v []byte) MiniLog {
	return m.Set(k, hex.EncodeToString(v))
}

func (m MiniLog) Set(k string, v interface{}) MiniLog {
	m[k] = v
	return m
}

func (m MiniLog) MarshalEvent(ev *zerolog.Event) {
	ev.Fields(m)
}

func (m MiniLog) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	// TODO: a solution without reflection?
	return json.Marshal(m)
}

func (m MiniLog) UnmarshalValue(v *fastjson.Value) error {
	return json.Unmarshal(v.MarshalTo(nil), &m)
}
