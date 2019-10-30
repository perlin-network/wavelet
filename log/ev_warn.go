package log

import (
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

type Warn struct {
	Message string `json:"message"`
}

func NewWarn(msg string) *Warn {
	return &Warn{
		Message: msg,
	}
}

func WarnTo(logger *zerolog.Logger, msg string) {
	EventTo(logger.Warn(), NewWarn(msg))
}

var _ JSONObject = (*Warn)(nil)

func (w *Warn) MarshalEvent(ev *zerolog.Event) {
	ev.Msg(w.Message)
}

func (w *Warn) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	o.Set("message", arena.NewString(w.Message))

	return o.MarshalTo(nil), nil
}

// UnmarshalValue does nothing.
func (w *Warn) UnmarshalValue(v *fastjson.Value) error {
	w.Message = string(v.GetStringBytes("message"))
	return nil
}
