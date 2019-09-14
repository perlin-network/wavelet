package wctl

import (
	"fmt"

	"github.com/valyala/fastjson"
)

// "{\\\"status\\\":\\\"Internal Server Error\\\",\\\"error\\\":\\\"error adding your transaction to graph: Node is currently ouf of sync. Please try again later.\\\"}\
type RequestError struct {
	Status      string `json:"status"`
	ErrorString string `json:"error"`

	RequestBody  []byte
	ResponseBody []byte
	StatusCode   int
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

/*
func (e *RequestError) UnmarshalJSON(b []byte) error {
	var parser fastjson.Parser

	v, err := parser.ParseBytes(b)
	if err != nil {
		return err
	}

	e.Status = string(v.GetStringBytes("status"))
	e.Error = string(v.GetStringBytes("error"))

	return nil
}
*/
