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

/*
	Message responses
*/

type MsgResponse struct {
	Message string `json:"msg"`
}

var _ log.JSONObject = (*MsgResponse)(nil)

func (s *MsgResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"msg", s.Message)
}

func (s *MsgResponse) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueAny(v, &s.Message, "msg")
}

func (s *MsgResponse) MarshalEvent(ev *zerolog.Event) {
	ev.Msg(s.Message)
}

type ErrResponse struct {
	Error          string `json:"error,omitempty"` // low-level runtime error
	HTTPStatusCode int    `json:"status"`          // http response status code
}

var _ log.JSONObject = (*ErrResponse)(nil)

func (e *ErrResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	return log.MarshalObjectBatch(arena,
		"status", http.StatusText(e.HTTPStatusCode),
		"error,omitempty", e.Error,
	)
}

func (e *ErrResponse) UnmarshalValue(v *fastjson.Value) error {
	return log.ValueBatch(v,
		"error", &e.Error,
		"status", &e.HTTPStatusCode)
}

func (e *ErrResponse) MarshalEvent(ev *zerolog.Event) {
	ev.Err(errors.New(e.Error))
	ev.Int("code", e.HTTPStatusCode)
	ev.Msg(e.Error)
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
