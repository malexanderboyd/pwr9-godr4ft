package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"log"
	"main/game"
	"net/http"
	"os"
	"strconv"
	"time"
)

type GameMessageType string

const NO_HOST_SENTINEL = -999
const (
	NewPlayer   GameMessageType = "new_player"
	ChatMessage GameMessageType = "chat_message"
	HostChange  GameMessageType = "host_change"
	GameStart   GameMessageType = "start_game"
	DeckContent GameMessageType = "deck_content"
)

type Message struct {
	Type GameMessageType `json:"type"`
	Data string          `json:"data"`
}

type GameDirector struct {
	clientsContents map[string][]string
	packs           map[string]SetPacks
	//
	port        int
	gameId      string
	options     game.GeneralOptions
	gameStarted bool
	host        int
	Clients     map[int]*Client
	messages    []*Message
	addClientCh chan *Client
	delClientCh chan *Client
	sendAllCh   chan *Message
	doneCh      chan bool
	errCh       chan error
}

func NewGameDirector(options game.GeneralOptions, port int, gameId string) *GameDirector {
	return &GameDirector{
		clientsContents: nil,
		packs:           nil,
		port:            port,
		gameId:          gameId,
		options:         nil,
		gameStarted:     false,
		host:            NO_HOST_SENTINEL,
		Clients:         make(map[int]*Client),
		messages:        []*Message{},
		addClientCh:     make(chan *Client),
		delClientCh:     make(chan *Client),
		sendAllCh:       make(chan *Message),
		doneCh:          make(chan bool),
		errCh:           make(chan error),
	}
}

func (director *GameDirector) addNewClient(c *Client) {
	director.addClientCh <- c
}

func (director *GameDirector) deleteClient(c *Client) {
	director.delClientCh <- c
}

func (director *GameDirector) shutdown(c *Client) {
	director.doneCh <- true
}

func (director *GameDirector) Error(err error) {
	director.errCh <- err
}

func (director *GameDirector) sendPastMessages(c *Client) {
	for _, msg := range director.messages {
		c.Write(msg)
	}
}

func (director *GameDirector) SendAll(msg *Message) {
	director.sendAllCh <- msg
}

func (director *GameDirector) sendAll(msg *Message) {
	for _, c := range director.Clients {
		c.Write(msg)
	}
}

func (director *GameDirector) sendHostMessage(msg *Message) {
	host := director.Clients[director.host]
	if host != nil {
		host.Write(msg)
	}
}

func (director *GameDirector) newClient(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		_, _ = fmt.Fprintf(w, err.Error())
	}

	defer func(ws *websocket.Conn) {
		err := ws.Close()
		if err != nil {
			director.errCh <- err
		}
	}(ws)

	client, err := NewClient(ws, director)
	if err != nil {
		if err = ws.Close(); err != nil {
			log.Println(err)
			_, _ = fmt.Fprintf(w, err.Error())
		}
	}
	director.addNewClient(client)
	if director.host == NO_HOST_SENTINEL {
		director.promoteNewHost()
	}
	client.Listen()
}

func (director *GameDirector) handleClientMessage(msg *Message) {
	switch msg.Type {
	case ChatMessage:
		director.SendAll(msg)
		break
	case GameStart:
		if !director.gameStarted {
			fmt.Println("Starting Game!")
			director.SendAll(msg)
			go director.startGame()
		}
		break
	default:
		break
	}
}

func (director *GameDirector) startGame() {
	director.gameStarted = true
	switch director.options.Type {
	case game.DRAFT:
		switch director.options.Mode {
		case game.CHAOS:
			break
		case game.CUBE:
			break
		case game.REGULAR:
			emp, _ := json.Marshal(director.packs)
			director.SendAll(&Message{
				Type: DeckContent,
				Data: string(emp),
			})

			break
		default:
			panic(fmt.Sprintf("Unknown game mode: %d", director.options.Mode))
		}
		break
	case game.SEALED:
		break
	default:
		panic(fmt.Sprintf("Unknown game type: %d", director.options.Type))
	}
}

func (director *GameDirector) pause() {
	fmt.Println("NO HOST! *PAUSING*.")
	ticks := 0
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ticker.C:
			ticks += 1
			if director.host != NO_HOST_SENTINEL {
				fmt.Println("NEW HOST! *UNPAUSING*.")
				ticker.Stop()
				break
			}
			if ticks == 6 {
				fmt.Println("Shutting down server. No host after 30 second grace period.")
				os.Exit(1)
			}
		}
	}
}

func (director *GameDirector) promoteNewHost() {
	if len(director.Clients) >= 1 {
		director.host = director.Clients[0].id
		director.sendHostMessage(&Message{
			Type: HostChange,
			Data: strconv.Itoa(1),
		})
	} else {
		director.host = NO_HOST_SENTINEL
		director.pause()
	}
}

type SetPacks struct {
	Packs [][]string `json:"packs"`
}

func (director *GameDirector) getGameResources() {
	switch director.options.Type {
	case game.DRAFT:
		switch director.options.Mode {
		case game.CHAOS:
			break
		case game.CUBE:
			break
		case game.REGULAR:
			opts := director.options.GameOptions.Draft.Regular
			var packs = make(map[string]SetPacks)
			for i := 0; i < opts.TotalPacks; i++ {
				setAbbrev := opts.SelectedPacks[strconv.Itoa(i)]
				res, err := http.Get(fmt.Sprintf("http://localhost:8000/set/%s/pack?n=3", setAbbrev))
				if err != nil {
					log.Fatalln("Cannot get game options!", err)
				}
				var boosters SetPacks
				msg, err := ioutil.ReadAll(res.Body)
				if err != nil {
					panic(err)
				}

				if err := json.Unmarshal(msg, &boosters); err != nil {
					panic(err)
				}
				packs[setAbbrev] = boosters
			}
			director.packs = packs
			break
		default:
			panic(fmt.Sprintf("Unknown game mode: %d", director.options.Mode))
		}
		break
	case game.SEALED:
		break
	default:
		panic(fmt.Sprintf("Unknown game type: %d", director.options.Type))
		break
	}

}

// We'll need to define an Upgrader
// this will require a Read and Write buffer size
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (director *GameDirector) Listen() {
	log.Println(fmt.Sprintf("[Game %s] Listening on port %d", director.gameId, director.port))
	// upgrade this connection to a WebSocket

	http.HandleFunc("/ws", director.newClient)
	log.Println("Created /ws handler")

	for {
		select {
		case c := <-director.addClientCh:
			log.Println("Added new client")
			director.Clients[c.id] = c
			log.Println("Total Connected", len(director.Clients))
			director.SendAll(&Message{
				Type: NewPlayer,
				Data: strconv.Itoa(len(director.Clients)),
			})
			director.sendPastMessages(c)
		case c := <-director.delClientCh:
			log.Println("Removing client: ", c.id)
			delete(director.Clients, c.id)
			if c.id == director.host {
				director.promoteNewHost()
			}
			director.SendAll(&Message{
				Type: NewPlayer,
				Data: strconv.Itoa(len(director.Clients)),
			})
		case msg := <-director.sendAllCh:
			log.Println("Sending to all clients: ", msg)
			director.messages = append(director.messages, msg)
			director.sendAll(msg)
		case err := <-director.errCh:
			log.Println("Error: ", err.Error())
		case <-director.doneCh:
			return
		}
	}

}

func main() {
	log.SetFlags(log.Lshortfile)

	port := flag.Int("port", 8000, "the port the server will open a socket server on")
	gameId := flag.String("gameId", "", "Four byte url safe hex string")
	flag.Parse()

	var GameOptions game.GeneralOptions
	res, err := http.Get(fmt.Sprintf("http://localhost:8000/game/%s", *gameId))
	if err != nil {
		log.Fatalln("Cannot get game options!", err)
	}

	defer res.Body.Close()

	if err := json.NewDecoder(res.Body).Decode(&GameOptions); err != nil {
		log.Fatalln(err)
	}

	director := NewGameDirector(GameOptions, *port, *gameId)

	director.getGameResources()

	go director.Listen()

	http.Handle("/", http.FileServer(http.Dir("webroot")))
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
