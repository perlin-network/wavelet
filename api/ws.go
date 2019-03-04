package api

import (
	"github.com/gorilla/websocket"
	"net/http"
	"time"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
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

func (s *sink) serve(w http.ResponseWriter, r *http.Request) error {
	filters := make(map[string]string)
	values := r.URL.Query()

	for key, queryKey := range s.filters {
		if queryValue := values.Get(queryKey); queryValue != "" {
			filters[key] = queryValue
		}
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}

	client := &client{filters: filters, sink: s, conn: conn, send: make(chan []byte, 256)}
	s.join <- client

	go client.readWorker()
	go client.writeWorker()

	return nil
}

type broadcastItem struct {
	fields map[string]interface{}
	buf    []byte
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
					if value, exists := msg.fields[key]; (exists && value != condition) || !exists {
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
