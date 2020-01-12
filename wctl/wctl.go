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

// Package wctl provides a client wrapper for the API.
package wctl

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet/cmd/wavelet/node"
	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
	"go.uber.org/atomic"
)

const (
	RouteLedger   = "/ledger"
	RouteAccount  = "/accounts"
	RouteContract = "/contract"
	RouteTxList   = "/tx"
	RouteTxSend   = "/tx/send"

	RouteNode       = "/node"
	RouteConnect    = RouteNode + "/connect"
	RouteDisconnect = RouteNode + "/disconnect"
	RouteRestart    = RouteNode + "/restart"

	ReqPost = "POST"
	ReqGet  = "GET"
)

const JSONPoolSize = 8 // sane size of 8 arenas and parsers in a pool

var ErrNoHost = errors.New("no host provided")

type Marshalable interface {
	Marshal() ([]byte, error)
}

type Config struct {
	APIHost    string
	APIPort    uint16
	APISecret  string
	PrivateKey edwards25519.PrivateKey
	UseHTTPS   bool
	Timeout    time.Duration

	// Optional
	Server *node.Wavelet
}

type Client struct {
	Config

	stdClient *http.Client

	edwards25519.PrivateKey
	edwards25519.PublicKey

	jsonPool fastjson.ParserPool
	url      string

	// Local state counters
	Block *atomic.Uint64

	// JSON stuff
	parsers fastjson.ParserPool
	arenas  fastjson.ArenaPool

	// Stop the background consensus that is created before
	stopConsensus func()

	// Stop other websockets that the user spawned
	stopSockets []func()

	// TODO: metrics, stake, consensus, network

	// These callbacks are only called when the function that calls them is
	// started. The function's name is written above each block.
	// These functions would also return a callback to stop polling.
	OnError

	// map of random number ID to function handlers
	handlers  map[int64]interface{}
	handlerMu sync.Mutex
}

func NewClient(config Config) (*Client, error) {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}

	if config.APIHost == "" {
		return nil, ErrNoHost
	}

	protocol := "http"
	if config.UseHTTPS {
		protocol = "https"
	}

	c := &Client{
		Config:     config,
		PrivateKey: config.PrivateKey,
		PublicKey:  config.PrivateKey.Public(),
		url: (&url.URL{
			Scheme: protocol,
			Host:   fmt.Sprintf("%s:%d", config.APIHost, config.APIPort),
		}).String(),
		stdClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		OnError: func(err error) {
			log.ErrNode(err, "")
		},
		Block: atomic.NewUint64(0),

		handlers: map[int64]interface{}{},
	}

	// Generate parsers and arenas
	for i := 0; i < JSONPoolSize; i++ {
		var a fastjson.Arena
		c.arenas.Put(&a)

		var p fastjson.Parser
		c.parsers.Put(&p)
	}

	ls, err := c.LedgerStatus()
	if err != nil {
		return c, errors.Wrap(err, "Failed to get self nonce")
	}

	c.Block.Store(ls.Block.Height)

	// Start listening to consensus to track Block
	cancel, err := c.pollConsensus()
	if err != nil {
		cancel()
		return nil, err
	}

	c.stopConsensus = cancel

	return c, nil
}

// AddHandler adds a custom handler into the list of handlers to call on a known
// event. The handler must have its only argument a pointer to a struct.
// Example:
//     func(finalized *wavelet.ConsensusFinalized)
func (c *Client) AddHandler(handler interface{}) func() {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()

	var id int64

	for {
		id = rand.Int63()
		if _, ok := c.handlers[id]; !ok {
			break
		}
	}

	c.handlers[id] = handler

	return func() {
		c.handlerMu.Lock()
		defer c.handlerMu.Unlock()

		delete(c.handlers, id)
	}
}

func (c *Client) Close() {
	c.stopConsensus()

	// cancel user-spawned sockets
	for _, c := range c.stopSockets {
		c()
	}
}

func (c *Client) GetContractCode(contractID string) (string, error) {
	path := fmt.Sprintf("%s/%s", RouteContract, contractID)

	res, err := c.Request(path, ReqGet, nil)
	return string(res), err
}

func (c *Client) GetContractPages(contractID string, index *uint64) (string, error) {
	path := fmt.Sprintf("%s/%s/page", RouteContract, contractID)
	if index != nil {
		path = fmt.Sprintf("%s/%d", path, *index)
	}

	res, err := c.Request(path, ReqGet, nil)
	return base64.StdEncoding.EncodeToString(res), err
}

// RequestJSON will make a request to a given path, with a given body and
// return the JSON bytes result into `out` to unmarshal.
func (c *Client) RequestJSON(path, method string, body log.MarshalableArena, out log.UnmarshalableValue) error {
	var bytes []byte

	if body != nil {
		a := c.arenas.Get()
		defer c.arenas.Put(a)

		raw, err := body.MarshalArena(a)
		if err != nil {
			return err
		}

		bytes = raw
	}

	resBody, err := c.Request(path, method, bytes)
	if err != nil {
		return err
	}

	if out == nil {
		return nil
	}

	p := c.parsers.Get()
	defer c.parsers.Put(p)

	v, err := p.ParseBytes(resBody)
	if err != nil {
		return errors.Wrap(err, "Failed to parse JSON")
	}

	return out.UnmarshalValue(v)
}

// Request will make a request to a given path, with a given body and return
// the result in raw bytes.
func (c *Client) Request(path string, method string, body []byte) ([]byte, error) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	addr := c.url + path

	req.URI().Update(addr)
	req.Header.SetMethod(method)
	req.Header.SetContentType("application/json")
	req.Header.Set("Authorization", "Bearer "+c.APISecret)

	if body != nil {
		req.SetBody(body)
	}

	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(res)

	if err := fasthttp.DoTimeout(req, res, 5*time.Second); err != nil {
		return nil, err
	}

	if res.StatusCode() != http.StatusOK {
		if err := ParseRequestError(res.Body()); err != nil {
			return nil, err
		}

		return nil, &RequestError{
			RequestBody:  req.Body(),
			ResponseBody: res.Body(),
			StatusCode:   res.StatusCode(),
		}
	}

	return res.Body(), nil
}

func (c *Client) error(err error) {
	if c.OnError != nil {
		c.OnError(err)
	}
}

// If this method returns true, skip the object
func (c *Client) possibleError(v *fastjson.Value) bool {
	// Return if not an error
	switch log.ValueString(v) {
	case "error":
		var ev log.ErrorEvent

		if err := ev.UnmarshalValue(v); err != nil {
			// Invalid error, log
			c.error(errors.Wrap(err, "Failed to parse error event"))
			return true
		}

		c.error(errors.Wrap(ev.Error, ev.Message))
		return true

	case "warn":
		// ???
		// TODO: Do something about this
		var ev log.WarnEvent

		if err := ev.UnmarshalValue(v); err != nil {
			c.error(errors.Wrap(err, "Failed to parse warn event"))
			return true
		}

		c.error(errors.New("WARNING: " + ev.Message))
		return true
	}

	return false
}

type MsgResponse struct {
	Message string `json:"msg"`
}

var _ log.JSONObject = (*MsgResponse)(nil)

func (r *MsgResponse) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	r.Message = string(v.GetStringBytes("msg"))
	return nil
}

func (r *MsgResponse) MarshalEvent(ev *zerolog.Event) {
	ev.Msg(r.Message)
}

func (r *MsgResponse) MarshalArena(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()
	o.Set("msg", arena.NewString(r.Message))

	return o.MarshalTo(nil), nil
}

func (r *MsgResponse) UnmarshalValue(v *fastjson.Value) error {
	r.Message = string(v.GetStringBytes("msg"))
	return nil
}

type RequestError struct {
	Status      string `json:"status"`
	ErrorString string `json:"error"`

	RequestBody  []byte
	ResponseBody []byte
	StatusCode   int

	underlying log.ErrorEvent
}

func ParseRequestError(b []byte) *RequestError {
	var parser fastjson.Parser
	var e RequestError

	v, err := parser.ParseBytes(b)
	if err != nil {
		return nil
	}

	e.Status = string(v.GetStringBytes("status"))
	e.ErrorString = string(v.GetStringBytes("error"))

	if e.Status == "" || e.ErrorString == "" {
		return nil
	}

	return &e
}

func (e *RequestError) Error() string {
	if e.ErrorString != "" {
		return e.ErrorString
	}

	return fmt.Sprintf(`Unexpected error code %d.
	Request body: %s
	Response body: %s`, e.StatusCode, e.RequestBody, e.ResponseBody)
}
