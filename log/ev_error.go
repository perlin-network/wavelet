package log

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

// !!! DOC: parse "event" for error|warn|debug first, then break it down

type ErrorEvent struct {
	Error   error  `json:"error"`
	Message string `json:"message"`

	// Only for unmarshalling, optional
	EventName string `json:"event,omitempty"`

	// For local node debugging only
	*zerolog.Event `json:"-"`

	logger *zerolog.Logger
}

func NewError(logger *zerolog.Logger) *ErrorEvent {
	return &ErrorEvent{
		Event:  zerolog.Dict(),
		logger: logger,
	}
}

func NewErrorFull(logger *zerolog.Logger, err error, msg string) *ErrorEvent {
	return &ErrorEvent{
		Error:   err,
		Message: msg,
		Event:   zerolog.Dict(),
		logger:  logger,
	}
}

var _ JSONObject = (*ErrorEvent)(nil)

/*
	Node logs
*/

func ErrNode(err error, msg string) {
	EventTo(node.Error(), NewErrorFull(nil, err, msg))
}

func ErrNodeX() *ErrorEvent {
	return NewError(&node)
}

func FatalNode(err error, msg string) {
	EventTo(node.Fatal(), NewErrorFull(nil, err, msg))
}

func PanicNode(err error, msg string) {
	EventTo(node.Panic(), NewErrorFull(nil, err, msg))
}

/*
	Normal error logs
*/

func Error(logger *zerolog.Logger, err error, msg string) {
	EventTo(logger.Error(), NewErrorFull(nil, err, msg))
}

func ErrorF(logger *zerolog.Logger, err error, msgF string, v ...interface{}) {
	EventTo(logger.Error(), NewErrorFull(nil, err, fmt.Sprintf(msgF, v...)))
}

func ErrorWithEvent(logger *zerolog.Logger, err error, msg string) *ErrorEvent {
	return NewErrorFull(logger, err, msg)
}

func (err *ErrorEvent) Send() {
	err.Msg("")
}

func (err *ErrorEvent) Err(setError error) *ErrorEvent {
	err.Error = setError
	return err
}

func (err *ErrorEvent) Msgf(f string, v ...interface{}) {
	err.Msg(fmt.Sprintf(f, v...))
}

func (err *ErrorEvent) Msg(msg string) {
	err.Message = msg
	err.MarshalEvent(err.logger.Error())
}

func (err *ErrorEvent) MarshalEvent(ev *zerolog.Event) {
	// Hijack MarshalEvent and log the keys into a debug node logger
	var debug = node.Debug()
	if err.Event != nil {
		debug.Dict("error", err.Event)
	}

	debug.Err(err.Error).Msg(err.Message)

	// Send the error over for real
	ev.Err(err.Error).Msg(err.Message)
}

func (err *ErrorEvent) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	o.Set("error", arena.NewString(err.Error.Error()))
	o.Set("message", arena.NewString(err.Message))

	return o.MarshalTo(nil), nil
}

func (err *ErrorEvent) UnmarshalValue(v *fastjson.Value) error {
	err.Message = string(v.GetStringBytes("message"))
	err.EventName = string(v.GetStringBytes("event"))

	if str := v.GetStringBytes("error"); len(str) > 0 {
		err.Error = errors.New(string(str))
	} else {
		err.Error = errors.New(err.Message)
	}

	return nil
}
