package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"time"
)

const channelBufSize = 100

var maxId = -1

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

// We'll need to define an Upgrader
// this will require a Read and Write buffer size
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	id       string
	director *GameDirector
	ws       *websocket.Conn
	ch       chan *Message
	messages []*Message
	doneCh   chan bool

	pool []SetCard
}

func NewClient(director *GameDirector) (*Client, error) {
	if director == nil {
		return nil, errors.New("cannot add client with nil GameDirector")
	}
	maxId++
	clientID := fmt.Sprintf("%s_%d", director.GameId, maxId)

	ch := make(chan *Message, channelBufSize)
	doneCh := make(chan bool)

	return &Client{clientID, director, nil, ch, []*Message{}, doneCh, nil}, nil
}

func (c *Client) Write(msg *Message) {
	select {
	case c.ch <- msg:
	default:
		c.director.deleteClient(c)
	}
}

func (c *Client) sendOldMessages() {
	for _, msg := range c.messages {
		c.Write(msg)
	}
}

func (c *Client) Listen() {
	go c.listenWrite()
	c.listenRead()
}

func (c *Client) listenRead() {
	defer func() {
		c.director.deleteClient(c)
		c.ws.Close()
	}()

	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error { c.ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	logger := GetLogger()
	logger.Infow("listening to read", "client", c.id)
	for {
		select {
		case <-c.doneCh:
			logger.Debugw("client done reading", "client", c.id)
			return
		default:
			var msg Message
			_, msgContent, err := c.ws.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					c.director.Error(err)
				}
				c.doneCh <- true
			}
			if err := json.Unmarshal(msgContent, &msg); err != nil {
				c.director.Error(err)
				c.doneCh <- true
			}
			c.director.handleClientMessage(c.id, &msg)
		}
	}
}

func (c *Client) listenWrite() {
	logger := GetLogger()
	logger.Debugw("listening to write", "client", c.id)
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case msg := <-c.ch:
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.ws.WriteJSON(msg)
			if err != nil {
				c.director.Error(err)
				c.doneCh <- true
			}
		case <-c.doneCh:
			logger.Debugw("client done writing", "client", c.id)
			return
		case <-ticker.C:
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.doneCh <- true
			}
		}
	}
	logger.Debugw("client done writing", "client", c.id)
}
