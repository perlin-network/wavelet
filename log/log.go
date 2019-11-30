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

type JSONRaw []byte

var _ JSONObject = (*JSONRaw)(nil)

func (j JSONRaw) MarshalEvent(ev *zerolog.Event) {
	ev.RawJSON("json", j)
	ev.Msg("Raw JSON")
}

func (j JSONRaw) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return j, nil
}

func (j JSONRaw) UnmarshalValue(v *fastjson.Value) error {
	var parser fastjson.Parser
	parsed, err := parser.ParseBytes(j)
	if err != nil {
		return err
	}

	*v = *parsed
	return nil
}
