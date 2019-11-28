package wctl

import (
	"net/http"
	"time"

	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
)

type MarshalableJSON interface {
	log.MarshalableArena
}

type UnmarshalableJSON interface {
	log.UnmarshalableValue
}

// RequestJSON will make a request to a given path, with a given body and
// return the JSON bytes result into `out` to unmarshal.
func (c *Client) RequestJSON(path, method string, body MarshalableJSON, out UnmarshalableJSON) error {
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
