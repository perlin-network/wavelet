package api

import (
	"github.com/fasthttp/websocket"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
	"strconv"
	"time"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var upgrader = websocket.FastHTTPUpgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
		return true
	},
}

type client struct {
	sink *sink
	conn *websocket.Conn

	filters map[string]string
	send    chan []byte
}

func (c *client) readWorker() {
	defer func() {
		c.sink.leave <- c
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { _ = c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (c *client) writeWorker() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			err := c.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				return
			}

			n := len(c.send)
			for i := 0; i < n; i++ {
				err := c.conn.WriteMessage(websocket.TextMessage, message)

				if err != nil {
					return
				}
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *sink) serve(ctx *fasthttp.RequestCtx) error {
	filters := make(map[string]string)
	ctx.QueryArgs().VisitAll(func(key, value []byte) {
		if string(value) != "" {
			filters[string(key)] = string(value)
		}
	})

	return upgrader.Upgrade(ctx, func(conn *websocket.Conn) {
		client := &client{filters: filters, sink: s, conn: conn, send: make(chan []byte, 256)}
		s.join <- client

		go client.readWorker()
		go client.writeWorker()

		// Block here because we need to keep the FastHTTPHandler active because of the way it works
		// Refer to https://github.com/fasthttp/websocket/issues/6
		select {}
	})
}

type broadcastItem struct {
	buf   []byte
	value *fastjson.Value
}

type sink struct {
	clients map[*client]struct{}
	filters map[string]string

	broadcast   chan broadcastItem
	join, leave chan *client
}

func (s *sink) run() {
	for {
		select {
		case client := <-s.join:
			s.clients[client] = struct{}{}
		case client := <-s.leave:
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				close(client.send)
			}
		case msg := <-s.broadcast:
		L:
			for client := range s.clients {
				for key, condition := range client.filters {
					o := msg.value.Get(key)
					if o != nil && valueEqual(o, condition) {
						continue L
					}
				}

				select {
				case client.send <- msg.buf:
				default:
					close(client.send)
					delete(s.clients, client)
				}
			}
		}
	}
}

func valueEqual(v *fastjson.Value, filter string) bool {
	switch v.Type() {
	case fastjson.TypeArray:
		fallthrough
	case fastjson.TypeNumber:
		fallthrough
	case fastjson.TypeObject:
		return string(v.MarshalTo(nil)) == filter
	case fastjson.TypeString:
		b, _ := v.StringBytes()
		return string(b) == filter
	case fastjson.TypeTrue, fastjson.TypeFalse:
		b, err := v.Bool()
		if err != nil {
			return false
		}
		return strconv.FormatBool(b) == filter
	default:
		return false
	}
}
