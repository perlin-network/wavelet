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

package wctl

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/fasthttp/websocket"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/valyala/fasthttp"
	"net/http"
	"net/url"
	"time"
)

type Config struct {
	APIHost    string
	APIPort    uint16
	PrivateKey edwards25519.PrivateKey
	UseHTTPS   bool
}

type Client struct {
	Config

	stdClient *http.Client

	edwards25519.PrivateKey
	edwards25519.PublicKey
}

func NewClient(config Config) (*Client, error) {
	stdClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	return &Client{Config: config, PrivateKey: config.PrivateKey, PublicKey: config.PrivateKey.Public(), stdClient: stdClient}, nil
}

// Request will make a request to a given path, with a given body and return result in out.
func (c *Client) RequestJSON(path string, method string, body MarshalableJSON, out UnmarshalableJSON) error {
	resBody, err := c.Request(path, method, body)
	if err != nil {
		return err
	}

	if out == nil {
		return nil
	}

	return out.UnmarshalJSON(resBody)
}

func (c *Client) Request(path string, method string, body MarshalableJSON) ([]byte, error) {
	protocol := "http"
	if c.Config.UseHTTPS {
		protocol = "https"
	}

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	addr := fmt.Sprintf("%s://%s:%d%s", protocol, c.Config.APIHost, c.Config.APIPort, path)

	req.URI().Update(addr)
	req.Header.SetMethod(method)
	req.Header.SetContentType("application/json")

	if body != nil {
		raw, err := body.MarshalJSON()
		if err != nil {
			return nil, err
		}

		req.SetBody(raw)
	}

	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(res)

	if err := fasthttp.DoTimeout(req, res, 5*time.Second); err != nil {
		return nil, err
	}

	if res.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code for query sent to %q: %d. request body: %q, response body: %q", addr, res.StatusCode(), req.Body(), res.Body())
	}

	return res.Body(), nil
}

// EstablishWS will create a websocket connection.
func (c *Client) EstablishWS(path string, query url.Values) (*websocket.Conn, error) {
	prot := "ws"
	if c.Config.UseHTTPS {
		prot = "wss"
	}

	host := fmt.Sprintf("%s:%d", c.Config.APIHost, c.Config.APIPort)
	uri := url.URL{Scheme: prot, Host: host, RawQuery: query.Encode(), Path: path}
	dialer := &websocket.Dialer{
		HandshakeTimeout: 3 * time.Second,
	}

	conn, _, err := dialer.Dial(uri.String(), nil)
	return conn, err
}

func (c *Client) PollLoggerSink(stop <-chan struct{}, sinkRoute string) (<-chan []byte, error) {
	if stop == nil {
		stop = make(chan struct{})
	}

	ws, err := c.EstablishWS(sinkRoute, url.Values{})
	if err != nil {
		return nil, err
	}

	evChan := make(chan []byte)

	go func() {
		defer close(evChan)

		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				return
			}

			select {
			case <-stop:
				return
			case evChan <- message:
			}
		}
	}()

	return evChan, nil
}

func (c *Client) PollAccounts(stop <-chan struct{}, accountID *string) (<-chan []byte, error) {
	v := url.Values{}
	if accountID != nil {
		v.Set("id", *accountID)
	}

	if stop == nil {
		stop = make(chan struct{})
	}

	ws, err := c.EstablishWS(RouteWSAccounts, v)
	if err != nil {
		return nil, err
	}

	evChan := make(chan []byte)

	go func() {
		defer close(evChan)

		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				return
			}

			select {
			case <-stop:
				return
			case evChan <- message:
			}
		}
	}()

	return evChan, nil
}

func (c *Client) PollContracts(stop <-chan struct{}, contractID *string) (<-chan []byte, error) {
	v := url.Values{}
	if contractID != nil {
		v.Set("id", *contractID)
	}

	if stop == nil {
		stop = make(chan struct{})
	}

	ws, err := c.EstablishWS(RouteWSContracts, v)
	if err != nil {
		return nil, err
	}

	evChan := make(chan []byte)

	go func() {
		defer close(evChan)

		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				return
			}

			select {
			case <-stop:
				return
			case evChan <- message:
			}
		}
	}()

	return evChan, nil
}

func (c *Client) PollTransactions(stop <-chan struct{}, txID *string, senderID *string, creatorID *string, tag *byte) (<-chan []byte, error) {
	v := url.Values{}
	if txID != nil {
		v.Set("tx_id", *txID)
	}
	if senderID != nil {
		v.Set("sender", *senderID)
	}
	if creatorID != nil {
		v.Set("creator", *creatorID)
	}
	if tag != nil {
		v.Set("tag", fmt.Sprintf("%x", *tag))
	}

	if stop == nil {
		stop = make(chan struct{})
	}

	ws, err := c.EstablishWS(RouteWSTransactions, v)
	if err != nil {
		return nil, err
	}

	evChan := make(chan []byte)

	go func() {
		defer close(evChan)

		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				return
			}

			select {
			case <-stop:
				return
			case evChan <- message:
			}
		}
	}()

	return evChan, nil
}

func (c *Client) GetLedgerStatus(senderID *string, creatorID *string, offset *uint64, limit *uint64) (LedgerStatusResponse, error) {
	path := fmt.Sprintf("%s?", RouteLedger)
	if senderID != nil {
		path = fmt.Sprintf("%ssender=%s&", path, *senderID)
	}
	if creatorID != nil {
		path = fmt.Sprintf("%screator=%s&", path, *creatorID)
	}
	if offset != nil {
		path = fmt.Sprintf("%soffset=%d&", path, *offset)
	}
	if limit != nil {
		path = fmt.Sprintf("%slimit=%d&", path, *limit)
	}

	var res LedgerStatusResponse
	err := c.RequestJSON(path, ReqGet, nil, &res)
	return res, err
}

func (c *Client) GetAccount(accountID string) (Account, error) {
	path := fmt.Sprintf("%s/%s", RouteAccount, accountID)

	var res Account
	err := c.RequestJSON(path, ReqGet, nil, &res)
	return res, err
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

func (c *Client) ListTransactions(senderID *string, creatorID *string, offset *uint64, limit *uint64) ([]Transaction, error) {
	path := fmt.Sprintf("%s?", RouteTxList)
	if senderID != nil {
		path = fmt.Sprintf("%ssender=%s&", path, *senderID)
	}
	if creatorID != nil {
		path = fmt.Sprintf("%screator=%s&", path, *creatorID)
	}
	if offset != nil {
		path = fmt.Sprintf("%soffset=%d&", path, *offset)
	}
	if limit != nil {
		path = fmt.Sprintf("%slimit=%d&", path, *limit)
	}

	var res TransactionList

	err := c.RequestJSON(path, ReqGet, nil, &res)
	return res, err

}

func (c *Client) GetTransaction(txID string) (Transaction, error) {
	path := fmt.Sprintf("%s/%s", RouteTxList, txID)

	var res Transaction
	err := c.RequestJSON(path, ReqGet, nil, &res)
	return res, err
}

func (c *Client) SendTransaction(tag byte, payload []byte) (SendTransactionResponse, error) {
	var res SendTransactionResponse

	var nonce [8]byte // TODO(kenta): nonce

	signature := edwards25519.Sign(c.PrivateKey, append(nonce[:], append([]byte{tag}, payload...)...))

	req := SendTransactionRequest{
		Sender:    hex.EncodeToString(c.PublicKey[:]),
		Tag:       tag,
		Payload:   hex.EncodeToString(payload),
		Signature: hex.EncodeToString(signature[:]),
	}

	err := c.RequestJSON(RouteTxSend, ReqPost, &req, &res)

	return res, err
}
