package api

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
)

// requestContext represents a context for a request.
type requestContext struct {
	service  *service
	response http.ResponseWriter
	request  *http.Request

	session *session
}

// readJSON decodes a HTTP requests JSON body into a struct.
func (c *requestContext) readJSON(out interface{}) error {
	r := io.LimitReader(c.request.Body, 4096*1024)
	defer c.request.Body.Close()

	data, err := ioutil.ReadAll(r)
	if err != nil {
		c.WriteJSON(http.StatusBadRequest, "bad request body")
		return err
	}

	if err = json.Unmarshal(data, out); err != nil {
		c.WriteJSON(http.StatusBadRequest, "malformed json")
		return err
	}
	return nil
}

// WriteJSON will write a given status code & JSON to a response.
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
func (c *requestContext) requireHeader(names ...string) string {
	for _, name := range names {
		values, ok := c.request.Header[name]

		if ok && len(values) > 0 {
			return values[0]
		}
	}

	c.WriteJSON(http.StatusBadRequest, "required header not found")
	return ""
}

// loadSession sets a session for a request.
func (c *requestContext) loadSession() bool {
	token := c.requireHeader(HeaderSessionToken, HeaderWebsocketProtocol)

	if err := validate.Var(token, "min=32,max=40"); err != nil {
		c.WriteJSON(http.StatusForbidden, "invalid session")
		return false
	}

	session, ok := c.service.registry.getSession(token)

	if !ok {
		c.WriteJSON(http.StatusForbidden, "session not found")
		return false
	}

	session.renew()

	c.session = session
	return true
}
