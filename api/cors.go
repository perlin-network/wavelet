package api

import (
	"github.com/valyala/fasthttp"
	"net/http"
	"strconv"
	"strings"
)

// Based on https://github.com/labstack/echo/tree/master/middleware.

type (
	// corsConfig defines the config for CORS middleware.
	corsConfig struct {
		// AllowOrigin defines a list of origins that may access the resource.
		// Optional. Default value []string{"*"}.
		allowOrigins []string

		// AllowMethods defines a list methods allowed when accessing the resource.
		// This is used in response to a preflight request.
		// Optional. Default value DefaultCORSConfig.AllowMethods.
		allowMethods []string

		// AllowHeaders defines a list of request headers that can be used when
		// making the actual request. This is in response to a preflight request.
		// Optional. Default value []string{}.
		allowHeaders []string

		// AllowCredentials indicates whether or not the response to the request
		// can be exposed when the credentials flag is true. When used as part of
		// a response to a preflight request, this indicates whether or not the
		// actual request can be made using credentials.
		// Optional. Default value false.
		allowCredentials bool

		// ExposeHeaders defines a whitelist headers that clients are allowed to
		// access.
		// Optional. Default value []string{}.
		exposeHeaders []string

		// MaxAge indicates how long (in seconds) the results of a preflight request
		// can be cached.
		// Optional. Default value 0.
		maxAge int
	}
)

var (
	// defaultCORSConfig is the default CORS middleware config.
	defaultCORSConfig = corsConfig{
		allowOrigins:     []string{"*"},
		allowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		allowHeaders:     []string{"*"},
		exposeHeaders:    []string{"Link"},
		allowCredentials: true,
		maxAge:           300,
	}
)

// CORS returns a Cross-Origin Resource Sharing (CORS) middleware.
// See: https://developer.mozilla.org/en/docs/Web/HTTP/Access_control_CORS
func cors() func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return corsWithConfig(defaultCORSConfig)
}

// corsWithConfig returns a CORS middleware with config.
// See: `CORS()`.
func corsWithConfig(config corsConfig) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	if len(config.allowOrigins) == 0 {
		config.allowOrigins = defaultCORSConfig.allowOrigins
	}
	if len(config.allowMethods) == 0 {
		config.allowMethods = defaultCORSConfig.allowMethods
	}

	allowMethods := strings.Join(config.allowMethods, ",")
	allowHeaders := strings.Join(config.allowHeaders, ",")
	exposeHeaders := strings.Join(config.exposeHeaders, ",")
	maxAge := strconv.Itoa(config.maxAge)

	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		fn := func(c *fasthttp.RequestCtx) {
			origin := string(c.Request.Header.Peek("Origin"))
			allowOrigin := ""

			// Check allowed origins
			for _, o := range config.allowOrigins {
				if o == "*" && config.allowCredentials {
					allowOrigin = origin
					break
				}
				if o == "*" || o == origin {
					allowOrigin = o
					break
				}
				if matchSubdomain(origin, o) {
					allowOrigin = origin
					break
				}
			}

			// Simple request
			if string(c.Method()) != http.MethodOptions {
				c.Response.Header.Add("Vary", "Origin")
				c.Response.Header.Set("Access-Control-Allow-Origin", allowOrigin)
				if config.allowCredentials {
					c.Response.Header.Set("Access-Control-Allow-Credentials", "true")
				}
				if exposeHeaders != "" {
					c.Response.Header.Set("Access-Control-Expose-Headers", exposeHeaders)
				}
				next(c)
				return
			}

			// Preflight request
			c.Response.Header.Add("Vary", "Origin")
			c.Response.Header.Add("Vary", "Access-Control-Request-Method")
			c.Response.Header.Add("Vary", "Access-Control-Request-Headers")
			c.Response.Header.Set("Access-Control-Allow-Origin", allowOrigin)
			c.Response.Header.Set("Access-Control-Allow-Methods", allowMethods)
			if config.allowCredentials {
				c.Response.Header.Set("Access-Control-Allow-Credentials", "true")
			}
			if allowHeaders != "" {
				c.Response.Header.Set("Access-Control-Allow-Headers", allowHeaders)
			} else {
				h := string(c.Response.Header.Peek("Access-Control-Request-Headers"))
				if h != "" {
					c.Response.Header.Set("Access-Control-Allow-Headers", h)
				}
			}
			if config.maxAge > 0 {
				c.Response.Header.Set("Access-Control-Max-Age", maxAge)
			}
			c.Response.SetStatusCode(http.StatusNoContent)
		}
		return fasthttp.RequestHandler(fn)
	}
}

func matchScheme(domain, pattern string) bool {
	didx := strings.Index(domain, ":")
	pidx := strings.Index(pattern, ":")
	return didx != -1 && pidx != -1 && domain[:didx] == pattern[:pidx]
}

func matchSubdomain(domain, pattern string) bool {
	if !matchScheme(domain, pattern) {
		return false
	}
	didx := strings.Index(domain, "://")
	pidx := strings.Index(pattern, "://")
	if didx == -1 || pidx == -1 {
		return false
	}
	domAuth := domain[didx+3:]
	// to avoid long loop by invalid long domain
	if len(domAuth) > 253 {
		return false
	}
	patAuth := pattern[pidx+3:]

	domComp := strings.Split(domAuth, ".")
	patComp := strings.Split(patAuth, ".")
	for i := len(domComp)/2 - 1; i >= 0; i-- {
		opp := len(domComp) - 1 - i
		domComp[i], domComp[opp] = domComp[opp], domComp[i]
	}
	for i := len(patComp)/2 - 1; i >= 0; i-- {
		opp := len(patComp) - 1 - i
		patComp[i], patComp[opp] = patComp[opp], patComp[i]
	}

	for i, v := range domComp {
		if len(patComp) <= i {
			return false
		}
		p := patComp[i]
		if p == "*" {
			return true
		}
		if p != v {
			return false
		}
	}
	return false
}
