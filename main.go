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
	"reflect"
	"strconv"
	"time"
)

type GameMessageType string

const (
	NewPlayer   GameMessageType = "new_player"
	ChatMessage GameMessageType = "chat_message"
	HostChange  GameMessageType = "host_change"
	GameStart   GameMessageType = "start_game"
	DeckContent GameMessageType = "deck_content"
)

type GameMessage struct {
	Type GameMessageType `json:"type"`
	Data string          `json:"data"`
}

type GameDirector struct {
	clients  []*websocket.Conn
	host     *websocket.Conn
	options game.GeneralOptions
	gameStarted bool
	clientsContents  map[string][]string
	packs map[string]SetPacks
}

func (director *GameDirector) newClient(w http.ResponseWriter, r *http.Request) {
	// upgrade this connection to a WebSocket
	newConnection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
	}
	director.clients = append(director.clients, newConnection)

	if len(director.clients) == 1 {
		director.promoteNewHost()
	}

	go director.handleClient(newConnection)
	director.announce(GameMessage{
		Type: NewPlayer,
		Data: strconv.Itoa(len(director.clients)),
	})
}

func (director *GameDirector) announce(message GameMessage) {
	for _, client := range director.clients {
		err := client.WriteJSON(message)
		if err != nil {
			panic(err)
		}
	}
}

func (director *GameDirector) announceToHost(message GameMessage) {
	if err := director.host.WriteJSON(message); err != nil {
		panic(err)
	}
}

func (director *GameDirector) handleClient(newConnection *websocket.Conn) {
	for {
		if msg_type, msgContent, err := newConnection.ReadMessage(); err != nil {
			err := newConnection.Close()
			if err != nil {
				panic(err)
			}
			director.removeClient(newConnection)
			break
		} else {
			fmt.Println(msg_type)

			var msg GameMessage
			if err := json.Unmarshal(msgContent, &msg); err != nil {
				panic(err)
			}
			fmt.Println(msg)
			switch msg.Type {
			case ChatMessage:
				director.announce(msg)
				break
			case GameStart:
				if !director.gameStarted {
					fmt.Println("Starting Game!")
					director.announce(msg)
					go director.startGame()
				}
				break
			default:
				break
			}
		}

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
			director.announce(GameMessage{
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
		break
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
			if director.host != nil {
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
	if len(director.clients) >= 1 {
		director.host = director.clients[0]
		director.announceToHost(GameMessage{
			Type: HostChange,
			Data: strconv.Itoa(1),
		})
	}
}

func (director *GameDirector) checkSwapHost(closedConn *websocket.Conn) {
	if reflect.DeepEqual(director.host, closedConn) {
		if len(director.clients) == 0 {
			director.host = nil
			go director.pause()
		} else {
			director.promoteNewHost()
		}
	}
}

func (director *GameDirector) removeClient(closedConnection *websocket.Conn) {
	for i, client := range director.clients {
		if reflect.DeepEqual(client, closedConnection) {
			director.clients = append(director.clients[:i], director.clients[i+1:]...)
			go director.checkSwapHost(closedConnection)
			break
		}
	}
	director.announce(GameMessage{
		Type: NewPlayer,
		Data: strconv.Itoa(len(director.clients)),
	})
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

func homePage(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Home Page")
}

func setupRoutes(director GameDirector) {
	http.HandleFunc("/", homePage)
	http.HandleFunc("/ws", director.newClient)
}

func main() {
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

	director := GameDirector{options: GameOptions, gameStarted: false}

	director.getGameResources()

	setupRoutes(director)
	fmt.Println("Game ", *gameId, ": Starting socket server on", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
