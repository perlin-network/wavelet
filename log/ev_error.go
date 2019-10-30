package log

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

// !!! DOC: parse "event" for error|warn|debug first, then break it down

type Error struct {
	Error   error  `json:"error"`
	Message string `json:"message"`
}

func NewError(err error, msg string) *Error {
	return &Error{
		Error:   err,
		Message: msg,
	}
}

func ErrNode(err error, msg string) {
	EventTo(node.Error(), NewError(err, msg))
}

func FatalNode(err error, msg string) {
	EventTo(node.Fatal(), NewError(err, msg))
}

func PanicNode(err error, msg string) {
	// Panic for debugging
	EventTo(node.Panic(), NewError(err, msg))
}

func ErrTo(logger *zerolog.Logger, err error, msg string) {
	EventTo(logger.Error(), NewError(err, msg))
}

func ErrToF(logger *zerolog.Logger, err error, msgF string, v ...interface{}) {
	EventTo(logger.Error(), NewError(err, fmt.Sprintf(msgF, v...)))
}

var _ JSONObject = (*Error)(nil)

func (err *Error) MarshalEvent(ev *zerolog.Event) {
	ev.Err(err.Error).Msg(err.Message)
}

func (err *Error) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	o.Set("error", arena.NewString(err.Error.Error()))
	o.Set("message", arena.NewString(err.Message))

	return o.MarshalTo(nil), nil
}

// UnmarshalValue does nothing.
func (err *Error) UnmarshalValue(v *fastjson.Value) error {
	if str := v.GetStringBytes("error"); len(str) > 0 {
		err.Error = errors.New(string(str))
	}

	err.Message = string(v.GetStringBytes("message"))
	return nil
}
