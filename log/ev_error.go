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
}

func NewError(logger *zerolog.Logger) *ErrorEvent {
	return &ErrorEvent{
		Event: logger.Error(),
	}
}

func NewErrorFull(logger *zerolog.Logger, err error, msg string) *ErrorEvent {
	return newErrorFull(logger.Error(), err, msg)
}

func newErrorFull(ev *zerolog.Event, err error, msg string) *ErrorEvent {
	return &ErrorEvent{
		Error:   err,
		Message: msg,
		Event:   ev,
	}
}

var _ JSONObject = (*ErrorEvent)(nil)

/*
	Node logs
*/

func ErrNode(err error, msg string) {
	NewErrorFull(&node, err, msg).Send()
}

func ErrNodeX() *ErrorEvent {
	return NewError(&node)
}

func FatalNode(err error, msg string) {
	newErrorFull(node.Fatal(), err, msg).Send()
}

func PanicNode(err error, msg string) {
	newErrorFull(node.Panic(), err, msg).Send()
}

/*
	Normal error logs
*/

func Error(logger *zerolog.Logger, err error, msg string) {
	NewErrorFull(logger, err, msg).Send()
}

func ErrorF(logger *zerolog.Logger, err error, msgF string, v ...interface{}) {
	NewErrorFull(logger, err, fmt.Sprintf(msgF, v...)).Send()
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
	if msg != "" {
		err.Message = msg
	}

	err.MarshalEvent(err.Event)
}

func (err *ErrorEvent) MarshalEvent(ev *zerolog.Event) {
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
