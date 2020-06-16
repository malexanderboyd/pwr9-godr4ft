package director

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/malexanderboyd/pwr9-godr4ft/internal"
	"github.com/malexanderboyd/pwr9-godr4ft/internal/director/models"
	"time"
)

var maxId = -1

type Client struct {
	Id        string
	director  *GameDirector
	Websocket *websocket.Conn
	ch        chan *models.Message
	messages  []*models.Message
	doneCh    chan bool
	pool      []models.SetCard
}

func NewClient(director *GameDirector) (*Client, error) {
	if director == nil {
		return nil, errors.New("cannot add client with nil GameDirector")
	}
	maxId++
	clientID := fmt.Sprintf("%s_%d", director.GameId, maxId)

	ch := make(chan *models.Message, models.ChannelBufSize)
	doneCh := make(chan bool)

	return &Client{clientID, director, nil, ch, []*models.Message{}, doneCh, nil}, nil
}

func (c *Client) Write(msg *models.Message) {
	select {
	case c.ch <- msg:
	default:
		c.Done()
	}
}

func (c *Client) Listen() {
	go c.listenWrite()
	c.listenRead()
}

func (c *Client) listenRead() {
	c.Websocket.SetReadLimit(models.MaxMessageSize)
	c.Websocket.SetReadDeadline(time.Now().Add(models.PongWait))
	c.Websocket.SetPongHandler(func(string) error { c.Websocket.SetReadDeadline(time.Now().Add(models.PongWait)); return nil })
	logger := internal.GetLogger()
	logger.Debugw("listening to read", "client", c.Id)
	for {
		select {
		case <-c.doneCh:
			logger.Debugw("client done reading", "client", c.Id)
			return
		default:
			var msg models.Message
			_, msgContent, err := c.Websocket.ReadMessage()
			if err != nil {
				if !websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
					c.director.Error(err)
				}
				c.Done()
			}
			if msgContent != nil {
				if err := json.Unmarshal(msgContent, &msg); err != nil {
					c.director.Error(err)
					c.Done()
				} else {
					c.director.HandleClientMessage(c.Id, &msg)
				}

			}
		}
	}
}

func (c *Client) listenWrite() {
	logger := internal.GetLogger()
	logger.Debugw("listening to write", "client", c.Id)
	ticker := time.NewTicker(models.PingPeriod)
	defer func() {
		ticker.Stop()
		c.Websocket.Close()
		c.director.DeleteClient(c)
		close(c.ch)
	}()
	for {
		select {
		case msg := <-c.ch:
			c.Websocket.SetWriteDeadline(time.Now().Add(models.WriteWait))
			err := c.Websocket.WriteJSON(msg)
			if err != nil {
				c.director.Error(err)
				c.Done()
			}
		case <-c.doneCh:
			logger.Debugw("client done writing", "client", c.Id)
			return
		case <-ticker.C:
			c.Websocket.SetWriteDeadline(time.Now().Add(models.WriteWait))
			if err := c.Websocket.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.Done()
			}
		}
	}
}

func (c *Client) Done() {
	c.doneCh <- true
}

func (c *Client) AddCardToPool(card models.SetCard) {
	c.pool = append(c.pool, card)
}

func (c *Client) WriteCurrentPool() {
	poolAsJson, err := json.Marshal(c.pool)
	if err != nil {
		c.director.Error(err)
	}
	c.Write(&models.Message{
		Type: models.PoolContent,
		Data: string(poolAsJson),
	})
}
