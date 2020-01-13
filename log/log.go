package log

import (
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

func init() { // nolint:gochecknoinits
	zerolog.MessageFieldName = "message"
	zerolog.LevelFieldName = "level"
	zerolog.ErrorFieldName = "error"

	setupChildLoggers()
}

type Loggable interface {
	UnmarshalableValue
	MarshalableEvent
}

type MarshalableEvent interface {
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

func LoggableTo(to *zerolog.Logger, level zerolog.Level, loggable MarshalableEvent) {
	EventTo(to.WithLevel(level), loggable)
}

func EventTo(ev *zerolog.Event, loggable MarshalableEvent) {
	loggable.MarshalEvent(ev)
}

func Info(logger *zerolog.Logger, loggable MarshalableEvent) {
	EventTo(logger.Info(), loggable)
}

type JSONRaw fastjson.Value

func ValueAsJSON(v *fastjson.Value) *JSONRaw {
	return (*JSONRaw)(v)
}

func (j *JSONRaw) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return (*fastjson.Value)(j).MarshalTo(nil), nil
}

func (j *JSONRaw) UnmarshalValue(v *fastjson.Value) error {
	*v = fastjson.Value(*j)
	return nil
}
