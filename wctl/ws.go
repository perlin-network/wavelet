package wctl

import (
	"fmt"
	"net/url"

	"github.com/gorilla/websocket"
)

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
func (c *Client) pollWS(path string, query url.Values, callback func([]byte)) (func(), error) {
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

type ErrInvalidPayload struct {
	FullJSON []byte
}

type ErrInvalidEvent struct {
	ErrInvalidPayload
	Event string
}

func errInvalidEvent(json []byte, event string) *ErrInvalidEvent {
	return &ErrInvalidEvent{
		ErrInvalidPayload: ErrInvalidPayload{
			FullJSON: json,
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

func errUnmarshallingFail(json []byte, key string, err error) *ErrUnmarshallingFail {
	return &ErrUnmarshallingFail{
		ErrInvalidPayload: ErrInvalidPayload{
			FullJSON: json,
		},
		Key:        key,
		Underlying: err,
	}
}

func (err *ErrUnmarshallingFail) Error() string {
	return "Error unmarshalling key " + err.Key + ": " + err.Underlying.Error()
}
