package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
)

const channelBufSize = 100

var maxId = -1

type Client struct {
	id int
	ws *websocket.Conn
	director *GameDirector
	ch chan*Message
	doneCh chan bool

	pool []SetCard
}

func NewClient(ws *websocket.Conn, director *GameDirector) (*Client, error) {
	if ws == nil {
		return nil, errors.New("cannot add client with nil websocket")
	}

	if director == nil {
		return nil, errors.New("cannot add client with nil GameDirector")
	}
	maxId++

	ch := make(chan *Message, channelBufSize)
	doneCh := make(chan bool)

	return &Client{maxId, ws, director, ch, doneCh, nil}, nil
}

func (c *Client) Write(msg *Message) {
	select {
		case c.ch <- msg:
		default:
			c.director.deleteClient(c)
			err := fmt.Errorf("client %d disconnected.", c.id)
			c.director.Error(err)
	}
}

func (c *Client) Listen() {
	go c.listenWrite()
	c.listenRead()
}

func(c *Client) listenRead() {
	logger := GetLogger()
	logger.Infow("listening to read", "client", c.id)
	for {
		select {
			case <- c.doneCh:
				c.director.deleteClient(c)
				c.doneCh <- true
				return

			default:
				var msg Message
				_, msgContent, err := c.ws.ReadMessage()
				if err != nil {
					c.director.Error(err)
				}
				if err := json.Unmarshal(msgContent, &msg); err != nil {
					c.director.deleteClient(c)
					c.doneCh <- true
					c.director.errCh <- err
				}
				c.director.handleClientMessage(c.id, &msg)
		}
	}
}


func(c *Client)  listenWrite() {
	logger := GetLogger()
	logger.Debugw("listening to write", "client", c.id)
	for {
		select {
			case msg := <-c.ch:
				logger.Debugw("send", "client", c.id, "msg", msg)
				err := c.ws.WriteJSON(msg)
				if err != nil {
					c.director.deleteClient(c)
					c.doneCh <- true
					c.director.errCh <- err
				}
			case <-c.doneCh:
				c.director.deleteClient(c)
				c.doneCh <- true
				return
		}
	}
}

