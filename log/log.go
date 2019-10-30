package log

import (
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

type Loggable interface {
	UnmarshalableValue

	// MarshalEvent sends the event as well.
	MarshalEvent(ev *zerolog.Event)
}

type MarshalableArena interface {
	MarshalArena(arena *fastjson.Arena) ([]byte, error)
}

type UnmarshalableValue interface {
	UnmarshalValue(v *fastjson.Value) error
}

type JSONObject interface {
	Loggable
	MarshalableArena
}

func LoggableTo(to zerolog.Logger, level zerolog.Level, loggable Loggable) {
	EventTo(to.WithLevel(level), loggable)
}

func EventTo(ev *zerolog.Event, loggable Loggable) {
	loggable.MarshalEvent(ev)
}

func Info(logger zerolog.Logger, loggable Loggable) {
	EventTo(logger.Info(), loggable)
}
