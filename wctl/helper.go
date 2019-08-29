package wctl

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

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
