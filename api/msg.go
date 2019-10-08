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

type marshalableJSON interface {
	marshalJSON(arena *fastjson.Arena) ([]byte, error)
}

var _ marshalableJSON = (*msgResponse)(nil)

type msgResponse struct {
	msg string
}

func (s *msgResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()

	o.Set("msg", arena.NewString(s.msg))

	return o.MarshalTo(nil), nil
}

type errResponse struct {
	Err            error `json:"-"` // low-level runtime error
	HTTPStatusCode int   `json:"-"` // http response status code
}

func (e *errResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()

	o.Set("status", arena.NewString(http.StatusText(e.HTTPStatusCode)))

	if e.Err != nil {
		o.Set("error", arena.NewString(e.Err.Error()))
	}

	return o.MarshalTo(nil), nil
}

func ErrBadRequest(err error) *errResponse {
	return &errResponse{
		Err:            err,
		HTTPStatusCode: http.StatusBadRequest,
	}
}

func ErrNotFound(err error) *errResponse {
	return &errResponse{
		Err:            err,
		HTTPStatusCode: http.StatusNotFound,
	}
}

func ErrInternal(err error) *errResponse {
	return &errResponse{
		Err:            err,
		HTTPStatusCode: http.StatusInternalServerError,
	}
}
