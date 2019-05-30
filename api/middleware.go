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
	"fmt"
	"github.com/valyala/fasthttp"
	"net/http"
	"os"
	"runtime/debug"
	"time"
)

type middleware func(fasthttp.RequestHandler) fasthttp.RequestHandler

func chain(f fasthttp.RequestHandler, middlewares []middleware) fasthttp.RequestHandler {
	last := f
	for i := len(middlewares) - 1; i >= 0; i-- {
		last = middlewares[i](last)
	}
	return last
}

func recoverer(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	fn := func(ctx *fasthttp.RequestCtx) {
		defer func() {
			if rvr := recover(); rvr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Panic: %+v\n", rvr)
				debug.PrintStack()

				ctx.Error(http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next(ctx)
	}

	return fasthttp.RequestHandler(fn)
}

func timeout(timeout time.Duration, msg string) func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return fasthttp.TimeoutHandler(next, timeout, msg)
	}
}
