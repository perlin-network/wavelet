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

func Recoverer(next fasthttp.RequestHandler) fasthttp.RequestHandler {
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

func Timeout(timeout time.Duration, msg string) func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return fasthttp.TimeoutHandler(next, timeout, msg)
	}
}
