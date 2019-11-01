package log

import (
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

type WarnEvent struct {
	Message string `json:"message"`
}

func NewWarn(msg string) *WarnEvent {
	return &WarnEvent{
		Message: msg,
	}
}

func Warn(logger *zerolog.Logger, msg string) {
	EventTo(logger.Warn(), NewWarn(msg))
}

var _ JSONObject = (*WarnEvent)(nil)

func (w *WarnEvent) MarshalEvent(ev *zerolog.Event) {
	ev.Msg(w.Message)
}

func (w *WarnEvent) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	o.Set("message", arena.NewString(w.Message))

	return o.MarshalTo(nil), nil
}

// UnmarshalValue does nothing.
func (w *WarnEvent) UnmarshalValue(v *fastjson.Value) error {
	w.Message = string(v.GetStringBytes("message"))
	return nil
}
