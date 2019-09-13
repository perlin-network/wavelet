package wctl

import (
	"fmt"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/valyala/fastjson"
)

// EstablishWS will create a websocket connection.
func (c *Client) EstablishWS(path string) (*websocket.Conn, error) {
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

	dialer := &websocket.Dialer{
		HandshakeTimeout: c.Config.Timeout,
	}

	conn, _, err := dialer.Dial(uri.String(), nil)
	return conn, err
}

// callback is spawned in a goroutine
func (c *Client) pollWS(path string, callback func(*fastjson.Value)) (func(), error) {
	ws, err := c.EstablishWS(path)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				c.OnError(err)
				return
			}

			go func(message []byte) {
				p := c.jsonPool.Get()

				o, err := p.ParseBytes(message)
				if err != nil {
					c.OnError(err)
					return
				}

				callback(o)

				c.jsonPool.Put(p)
			}(message)
		}
	}()

	return func() {
		// Also kills the for loop above
		ws.Close()
	}, nil
}

type ErrInvalidPayload struct {
	JSONValue string
}

type ErrMismatchMod struct {
	ErrInvalidPayload
	WantedMod string
	GotMod    string
}

func checkMod(v *fastjson.Value, mod string) error {
	if string(v.GetStringBytes("mod")) != mod {
		return errMismatchMod(v, mod)
	}

	return nil
}

func errMismatchMod(v *fastjson.Value, wantedMod string) *ErrMismatchMod {
	return &ErrMismatchMod{
		ErrInvalidPayload: ErrInvalidPayload{
			JSONValue: v.String(),
		},
		WantedMod: wantedMod,
		GotMod:    string(v.GetStringBytes("mod")),
	}
}

func (err *ErrMismatchMod) Error() string {
	return "Mismatched \"mod\" field in JSON: expected " + err.WantedMod +
		", got " + err.GotMod
}

type ErrInvalidEvent struct {
	ErrInvalidPayload
	Event string
}

func errInvalidEvent(v *fastjson.Value, event string) *ErrInvalidEvent {
	return &ErrInvalidEvent{
		ErrInvalidPayload: ErrInvalidPayload{
			JSONValue: v.String(),
		},
		Event: event,
	}
}

func (err *ErrInvalidEvent) Error() string {
	return "Unsupported event: " + err.Event
}

type ErrUnmarshallingFail struct {
	ErrInvalidPayload
	Key        string
	Underlying error
}

func errUnmarshallingFail(v *fastjson.Value, key string, err error) *ErrUnmarshallingFail {
	return &ErrUnmarshallingFail{
		ErrInvalidPayload: ErrInvalidPayload{
			JSONValue: v.String(),
		},
		Key:        key,
		Underlying: err,
	}
}

func (err *ErrUnmarshallingFail) Error() string {
	return "Error unmarshalling key " + err.Key + ": " + err.Underlying.Error()
}
