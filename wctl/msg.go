package wctl

import (
	"github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

type MsgResponse struct {
	Message string `json:"msg"`
}

var _ log.JSONObject = (*MsgResponse)(nil)

func (r *MsgResponse) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	r.Message = string(v.GetStringBytes("msg"))
	return nil
}

func (r *MsgResponse) MarshalEvent(ev *zerolog.Event) {
	ev.Msg(r.Message)
}

func (r *MsgResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	o.Set("msg", arena.NewString(r.Message))

	return o.MarshalTo(nil), nil
}

func (r *MsgResponse) UnmarshalValue(v *fastjson.Value) error {
	r.Message = string(v.GetStringBytes("msg"))
	return nil
}
