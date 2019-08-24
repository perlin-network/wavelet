package wctl

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

// RequestJSON will make a request to a given path, with a given body and
// return the JSON bytes result into `out` to unmarshal.
func (c *Client) RequestJSON(path string, method string, body MarshalableJSON, out UnmarshalableJSON) error {
	raw, err := body.MarshalJSON()
	if err != nil {
		return err
	}

	resBody, err := c.Request(path, method, raw)
	if err != nil {
		return err
	}

	if out == nil {
		return nil
	}

	return out.UnmarshalJSON(resBody)
}

// Request will make a request to a given path, with a given body and return
// the result in raw bytes.
func (c *Client) Request(path string, method string, body []byte) ([]byte, error) {
	protocol := "http"
	if c.UseHTTPS {
		protocol = "https"
	}

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	addr := fmt.Sprintf(
		"%s://%s:%d%s",
		protocol, c.APIHost, c.APIPort, path,
	)

	req.URI().Update(addr)
	req.Header.SetMethod(method)
	req.Header.SetContentType("application/json")

	if body != nil {
		req.SetBody(body)
	}

	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(res)

	if err := fasthttp.DoTimeout(req, res, 5*time.Second); err != nil {
		return nil, err
	}

	if res.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf(
			"unexpected status code for query sent to %q: %d. request body: %q, response body: %q",
			addr, res.StatusCode(), req.Body(), res.Body(),
		)
	}

	return res.Body(), nil
}

// EstablishWS will create a websocket connection.
func (c *Client) EstablishWS(path string, query url.Values) (*websocket.Conn, error) {
	prot := "ws"
	if c.UseHTTPS {
		prot = "wss"
	}

	host := fmt.Sprintf("%s:%d", c.APIHost, c.APIPort)
	uri := url.URL{
		Scheme: prot,
		Host:   host,
		Path:   path,
	}

	if query != nil {
		uri.RawQuery = query.Encode()
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: c.Config.Timeout,
	}

	conn, _, err := dialer.Dial(uri.String(), nil)
	return conn, err
}

func (c *Client) pollWS(stop <-chan struct{}, ev chan []byte, path string, query url.Values) error {
	if stop == nil {
		stop = make(chan struct{})
	}

	ws, err := c.EstablishWS(path, query)
	if err != nil {
		return err
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

	return nil
}

func jsonHex(v *fastjson.Value, dst []byte, keys ...string) error {
	_, err := hex.Decode(dst, v.GetStringBytes(keys...))
	return err
}
