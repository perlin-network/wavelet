// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package api

import (
	"net/http"

	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
)

type MarshalableArena interface {
	MarshalArena(arena *fastjson.Arena) ([]byte, error)
}

type UnmarshalableValue interface {
	UnmarshalValue(v *fastjson.Value) error
}

type JSONObject interface {
	MarshalableArena
	UnmarshalableValue
	log.Loggable
}

/*
	Message responses
*/

type MsgResponse struct {
	Message string `json:"msg"`
}

var _ JSONObject = (*MsgResponse)(nil)

func (s *MsgResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	o.Set("msg", arena.NewString(s.Message))

	return o.MarshalTo(nil), nil
}

func (s *MsgResponse) UnmarshalValue(v *fastjson.Value) error {
	s.Message = valueString(v, "msg")
	return nil
}

func (s *MsgResponse) MarshalEvent(ev *zerolog.Event) error {
	ev.Str("msg", s.Message)
	return nil
}

type ErrResponse struct {
	Error          string `json:"error,omitempty"` // low-level runtime error
	HTTPStatusCode int    `json:"status"`          // http response status code
}

var _ JSONObject = (*ErrResponse)(nil)

func (e *ErrResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()

	o.Set("status", arena.NewString(http.StatusText(e.HTTPStatusCode)))

	if e.Error != "" {
		o.Set("error", arena.NewString(e.Error))
	}

	return o.MarshalTo(nil), nil
}

func (e *ErrResponse) UnmarshalValue(v *fastjson.Value) error {
	e.Error = valueString(v, "error")
	e.HTTPStatusCode = v.GetInt("status")
	return nil
}

func (e *ErrResponse) MarshalEvent(ev *zerolog.Event) error {
	ev.Err(errors.New(e.Error))
	ev.Int("code", e.HTTPStatusCode)
	return nil
}

func ErrBadRequest(err error) *ErrResponse {
	return &ErrResponse{
		Error:          err.Error(),
		HTTPStatusCode: http.StatusBadRequest,
	}
}

func ErrNotFound(err error) *ErrResponse {
	return &ErrResponse{
		Error:          err.Error(),
		HTTPStatusCode: http.StatusNotFound,
	}
}

func ErrInternal(err error) *ErrResponse {
	return &ErrResponse{
		Error:          err.Error(),
		HTTPStatusCode: http.StatusInternalServerError,
	}
}
