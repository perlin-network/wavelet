package wctl

import (
	"encoding/hex"
	"errors"
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
	var bytes []byte

	if body != nil {
		raw, err := body.MarshalJSON()
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

	if query != nil && len(query) > 0 {
		uri.RawQuery = query.Encode()
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: c.Config.Timeout,
	}

	conn, _, err := dialer.Dial(uri.String(), nil)
	return conn, err
}

// callback is spawned in a goroutine
func (c *Client) pollWS(callback func([]byte), path string, query url.Values) (func(), error) {
	ws, err := c.EstablishWS(path, query)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				return
			}

			go callback(message)
		}
	}()

	return func() {
		// Also kills the for loop above
		ws.Close()
	}, nil
}

var ErrInvalidHexLength = errors.New("Invalid hex bytes length")

func jsonHex(v *fastjson.Value, dst []byte, keys ...string) error {
	//_, fn, line, _ := runtime.Caller(1)
	//fmt.Println(fn, line)

	i, err := hex.Decode(dst, v.GetStringBytes(keys...))
	if err != nil {
		return err
	}

	if i != len(dst) {
		return ErrInvalidHexLength
	}

	return nil
}

func StringIDs(ids [][32]byte) []string {
	s := make([]string, len(ids))
	for i := range ids {
		s[i] = hex.EncodeToString(ids[i][:])
	}
	return s
}
