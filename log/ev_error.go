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
}

func NewError(err error, msg string) *ErrorEvent {
	return &ErrorEvent{
		Error:   err,
		Message: msg,
	}
}

var _ JSONObject = (*ErrorEvent)(nil)

func ErrNode(err error, msg string) {
	EventTo(node.Error(), NewError(err, msg))
}

func ErrNodeX() *zerolog.Event {
	return node.Error()
}

func FatalNode(err error, msg string) {
	EventTo(node.Fatal(), NewError(err, msg))
}

func PanicNode(err error, msg string) {
	// Panic for debugging
	EventTo(node.Panic(), NewError(err, msg))
}

func Error(logger *zerolog.Logger, err error, msg string) {
	EventTo(logger.Error(), NewError(err, msg))
}

func ErrorF(logger *zerolog.Logger, err error, msgF string, v ...interface{}) {
	EventTo(logger.Error(), NewError(err, fmt.Sprintf(msgF, v...)))
}

func ErrorX(logger *zerolog.Logger, err error) *zerolog.Event {
	ev := logger.Error()
	if err != nil {
		ev.Err(err)
	}

	return ev
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

// UnmarshalValue does nothing.
func (err *ErrorEvent) UnmarshalValue(v *fastjson.Value) error {
	if str := v.GetStringBytes("error"); len(str) > 0 {
		err.Error = errors.New(string(str))
	}

	err.Message = string(v.GetStringBytes("message"))
	return nil
}
