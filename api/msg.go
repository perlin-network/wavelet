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

	"github.com/valyala/fastjson"
)

type MarshalableJSON interface {
	MarshalJSON(arena *fastjson.Arena) ([]byte, error)
}

var _ MarshalableJSON = (*MsgResponse)(nil)

type MsgResponse struct {
	Message string `json:"msg"`
}

func (s *MsgResponse) MarshalJSON(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	o.Set("msg", arena.NewString(s.Message))

	return o.MarshalTo(nil), nil
}

type ErrResponse struct {
	Err            error `json:"error,omitempty"` // low-level runtime error
	HTTPStatusCode int   `json:"status"`          // http response status code
}

func (e *ErrResponse) MarshalJSON(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()

	o.Set("status", arena.NewString(http.StatusText(e.HTTPStatusCode)))

	if e.Err != nil {
		o.Set("error", arena.NewString(e.Err.Error()))
	}

	return o.MarshalTo(nil), nil
}

func ErrBadRequest(err error) *ErrResponse {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: http.StatusBadRequest,
	}
}

func ErrNotFound(err error) *ErrResponse {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: http.StatusNotFound,
	}
}

func ErrInternal(err error) *ErrResponse {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: http.StatusInternalServerError,
	}
}
