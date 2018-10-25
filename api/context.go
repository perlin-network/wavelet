package api

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

// requestContext represents a context for a request.
type requestContext struct {
	service  *service
	response http.ResponseWriter
	request  *http.Request

	session *session
}

// readJSON decodes a HTTP requests JSON body into a struct.
// Can call this once per request
func (c *requestContext) readJSON(out interface{}) error {
	r := io.LimitReader(c.request.Body, MaxRequestBodySize)
	defer c.request.Body.Close()

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "bad request body")
	}

	if err = json.Unmarshal(data, out); err != nil {
		return errors.Wrap(err, "malformed json")
	}
	return nil
}

// WriteJSON will write a given status code & JSON to a response.
// Should call this once per request
func (c *requestContext) WriteJSON(status int, data interface{}) {
	out, err := json.Marshal(data)
	if err != nil {
		c.WriteJSON(http.StatusInternalServerError, "server error")
		return
	}
	c.response.Header().Set("Content-Type", "application/json")
	c.response.WriteHeader(status)
	c.response.Write(out)
}

// requireHeader returns a header value if presents or stops request with a bad request response.
func (c *requestContext) requireHeader(names ...string) (string, error) {
	for _, name := range names {
		values, ok := c.request.Header[name]

		if ok && len(values) > 0 {
			return values[0], nil
		}
	}

	return "", errors.New("required header not found")
}

// loadSession sets a session for a request.
func (c *requestContext) loadSession() error {
	token, err := c.requireHeader(HeaderSessionToken, HeaderWebsocketProtocol)
	if err != nil {
		return err
	}

	if err := validate.Var(token, "min=32,max=40"); err != nil {
		return errors.Wrap(err, "invalid session")
	}

	session, ok := c.service.registry.getSession(token)
	if !ok {
		return errors.New("session not found")
	}

	session.renew()

	c.session = session
	return nil
}
