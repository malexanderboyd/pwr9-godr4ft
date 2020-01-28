package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
)

const channelBufSize = 100

var maxId int = 0

type Client struct {
	id int
	ws *websocket.Conn
	director *GameDirector
	ch chan*Message
	doneCh chan bool
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

	return &Client{maxId, ws, director, ch, doneCh}, nil
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
	log.Println("Listening read from client", c.id)
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
					panic(err)
				}
				c.director.handleClientMessage(&msg)
		}
	}
}


func(c *Client)  listenWrite() {
	log.Println("Listening write to client", c.id)
	for {
		select {
			case msg := <-c.ch:
				log.Println("Send:", msg)
				err := c.ws.WriteJSON(msg)
				if err != nil {
					panic(err)
				}
			case <-c.doneCh:
				c.director.deleteClient(c)
				c.doneCh <- true
				return
		}
	}
}

