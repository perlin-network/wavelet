package wctl

import (
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type MarshalableJSON interface {
	MarshalJSON() ([]byte, error)
}

type UnmarshalableJSON interface {
	UnmarshalJSON([]byte) error
}

// RequestJSON will make a request to a given path, with a given body and
// return the JSON bytes result into `out` to unmarshal.
func (c *Client) RequestJSON(path, method string, body MarshalableJSON, out UnmarshalableJSON) error {
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
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	addr := c.url + path

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

type jsonRaw []byte

func (j jsonRaw) MarshalJSON() ([]byte, error) {
	return j, nil
}

var ErrInvalidHexLength = errors.New("Invalid hex bytes length")

func jsonHex(v *fastjson.Value, dst []byte, keys ...string) error {
	//_, fn, line, _ := runtime.Caller(1)
	//fmt.Println(fn, line)

	i, err := hex.Decode(dst, v.GetStringBytes(keys...))
	if err != nil {
		return errUnmarshallingFail(v, strings.Join(keys, "."), err)
	}

	if i != len(dst) {
		return errUnmarshallingFail(v, strings.Join(keys, "."),
			ErrInvalidHexLength)
	}

	return nil
}

func jsonTime(v *fastjson.Value, t *time.Time, keys ...string) error {
	Time, err := time.Parse(time.RFC3339, string(v.GetStringBytes(keys...)))
	if err != nil {
		return errUnmarshallingFail(v, strings.Join(keys, "."), err)
	}

	*t = Time
	return nil
}

func jsonString(v *fastjson.Value, keys ...string) string {
	return string(v.GetStringBytes(keys...))
}

func StringIDs(ids [][32]byte) []string {
	s := make([]string, len(ids))
	for i := range ids {
		s[i] = hex.EncodeToString(ids[i][:])
	}
	return s
}
