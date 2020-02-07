package main

import (
	"container/list"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
)

const channelBufSize = 100

var maxId = -1

type Client struct {
	id int
	ws *websocket.Conn
	director *GameDirector
	ch chan*Message
	doneCh chan bool

	packs *list.List
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

	return &Client{maxId, ws, director, ch, doneCh, list.New()}, nil
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
	log.Println(fmt.Sprintf("[Client: %d] listening to read:", c.id))
	for {
		select {
			case <- c.doneCh:
				c.director.deleteClient(c)
				c.doneCh <- true
				return

			default:
				var msg Message
				messageType, msgContent, err := c.ws.ReadMessage()
				if err != nil {
					c.director.Error(err)
				}
				fmt.Println(messageType)
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
	log.Println(fmt.Sprintf("[Client: %d] Listening to write", c.id))
	for {
		select {
			case msg := <-c.ch:
				log.Println(fmt.Sprintf("[Client: %d] Send:", c.id), msg)
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

