package main

import (
	"encoding/json"
	"errors"
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
	ChooseCard  GameMessageType = "choose_card"
)

type Message struct {
	Type GameMessageType `json:"type"`
	Data string          `json:"data"`
}

type DraftRound struct {
	SetAbbreviation string
	PlayerPacks     map[int][]SetCard
}

type GameDirector struct {
	clientsContents map[string][]string
	pool            []string
	//
	port              int
	gameId            string
	options           game.GeneralOptions
	gameStarted       bool
	packNumber        int
	round             int
	roundTimerType    string
	roundPacks        map[int]DraftRound
	roundPicksTimerCh chan int
	totalPacks        int
	host              int
	Clients           map[int]*Client
	Seats             map[int]int
	messages          []*Message
	addClientCh       chan *Client
	delClientCh       chan *Client
	sendAllCh         chan *Message
	doneCh            chan bool
	errCh             chan error
}

func NewGameDirector(options game.GeneralOptions, port int, gameId string) *GameDirector {
	return &GameDirector{
		clientsContents:   nil,
		roundPacks:        make(map[int]DraftRound),
		pool:              nil,
		port:              port,
		gameId:            gameId,
		options:           options,
		gameStarted:       false,
		packNumber:        0,
		roundTimerType:    "",
		round:             0,
		roundPicksTimerCh: nil,
		Seats:             make(map[int]int),
		totalPacks:        0,
		host:              NO_HOST_SENTINEL,
		Clients:           make(map[int]*Client),
		messages:          []*Message{},
		addClientCh:       make(chan *Client),
		delClientCh:       make(chan *Client),
		sendAllCh:         make(chan *Message),
		doneCh:            make(chan bool),
		errCh:             make(chan error),
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
		director.Error(err)
		_, _ = fmt.Fprintf(w, err.Error())
	}

	client, err := NewClient(ws, director)
	if err != nil {
		if err = ws.Close(); err != nil {
			director.Error(err)
			_, _ = fmt.Fprintf(w, err.Error())
		}
	}
	if director.host == NO_HOST_SENTINEL {
		director.host = client.id
		client.Write(&Message{
			Type: HostChange,
			Data: strconv.Itoa(1),
		})
	}
	director.addNewClient(client)
	go client.Listen()
}

func (director *GameDirector) handleClientMessage(clientID int, msg *Message) {
	logger := GetLogger()
	switch msg.Type {
	case ChatMessage:
		director.SendAll(msg)
		break
	case GameStart:
		if !director.gameStarted {

			type TimerSettings struct {
				Type string `json:"timer"`
			}
			var timerSetting = &TimerSettings{}
			if err := json.Unmarshal([]byte(msg.Data), &timerSetting); err != nil {
				director.Error(err)
			}
			director.roundTimerType = timerSetting.Type
			logger.Infow("Starting Game!")
			director.SendAll(msg)
			go director.startGame()
		}
		break
	case ChooseCard:
		if director.gameStarted {
			if err := director.handleClientChooseCard(clientID, msg); err != nil {
				director.Error(err)
			}
		}
		break
	default:
		break
	}
}

func (director *GameDirector) handleClientChooseCard(clientID int, msg *Message) error {
	rawMsgContents := msg.Data
	client := director.Clients[clientID]
	if client == nil {
		return errors.New(fmt.Sprintf("No client with id: %d. Must provide valid client ID", clientID))
	} else {

		type ChooseCardJson struct {
			PickedCardIndex int `json:"pickedCardIndex"`
		}

		var selectedCardMsg ChooseCardJson
		if err := json.Unmarshal([]byte(rawMsgContents), &selectedCardMsg); err != nil {
			director.Error(err)
		}

		playerSeat := director.Seats[client.id]
		currentPack := director.roundPacks[director.packNumber].PlayerPacks[playerSeat]

		if selectedCardMsg.PickedCardIndex >= len(currentPack) || selectedCardMsg.PickedCardIndex < 0 {
			return errors.New(fmt.Sprintf("[client %d] chose an invalid card index %d", clientID, selectedCardMsg.PickedCardIndex))
		}

		chosenCard := currentPack[selectedCardMsg.PickedCardIndex]

		currentPack = append(currentPack[:selectedCardMsg.PickedCardIndex], currentPack[selectedCardMsg.PickedCardIndex+1:]...)

		client.pool.PushBack(chosenCard)

		var nextClientSeat int
		if director.packNumber%2 == 0 {
			// rounds go left, right, left ...
			if playerSeat+1 >= len(director.Seats) {
				nextClientSeat = 0
			} else {
				nextClientSeat = playerSeat + 1
			}
		} else {
			if playerSeat-1 <= 0 {
				nextClientSeat = len(director.Seats) - 1
			} else {
				nextClientSeat = playerSeat - 1
			}
		}

		director.roundPicksTimerCh <- 1
		director.roundPacks[director.packNumber].PlayerPacks[nextClientSeat] = currentPack

	}
	return nil
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
			CurrentRound := director.roundPacks[director.packNumber]
			for i, client := range director.Clients {

				if i >= len(CurrentRound.PlayerPacks) {
					break
				}

				playerPack := CurrentRound.PlayerPacks[i]
				director.Seats[i] = client.id
				type CardPack struct {
					SetName string    `json:"setName"`
					Pack    []SetCard `json:"pack"`
				}

				emp, _ := json.Marshal(&CardPack{
					SetName: CurrentRound.SetAbbreviation,
					Pack:    playerPack,
				})
				go client.Write(&Message{
					Type: DeckContent,
					Data: string(emp),
				})

			}
			break
		default:
			panic(fmt.Sprintf("Unknown game mode: %d", director.options.Mode))
		}
		director.roundPicksTimerCh = director.startRoundPicksTimer()
		break
	case game.SEALED:
		break
	default:
		panic(fmt.Sprintf("Unknown game type: %d", director.options.Type))
	}
}

func (director *GameDirector) startNextPack() {
	logger := GetLogger()
	director.round = 0
	director.packNumber += 1
	if _, ok := director.roundPacks[director.packNumber]; !ok {
		logger.Infow("END OF GAME!")
		os.Exit(0)
	}
}

func (director *GameDirector) startNextRound() {
	director.round += 1
	CurrentPackRound := director.roundPacks[director.packNumber]
	// TODO Fix this to properly roundrobin
	var totalEmptys = 0
	for _, pack := range CurrentPackRound.PlayerPacks {
		if len(pack) == 0 {
			totalEmptys += 1
		} else {
			break
		}

	}
	if totalEmptys == len(director.Seats) {
		director.startNextPack()
		CurrentPackRound = director.roundPacks[director.packNumber]
	}

	for i, client := range director.Clients {

		if i >= len(CurrentPackRound.PlayerPacks) {
			break
		}
		playerSeat := director.Seats[client.id]
		playerPack := CurrentPackRound.PlayerPacks[playerSeat]

		type CardPack struct {
			SetName string    `json:"setName"`
			Pack    []SetCard `json:"pack"`
		}

		emp, _ := json.Marshal(&CardPack{
			SetName: CurrentPackRound.SetAbbreviation,
			Pack:    playerPack,
		})
		go client.Write(&Message{
			Type: DeckContent,
			Data: string(emp),
		})

	}
	director.roundPicksTimerCh = director.startRoundPicksTimer()
}

func (director *GameDirector) startRoundPicksTimer() chan int {
	var roundTime time.Duration
	switch director.roundTimerType {
	case "leisurely":
		//'Leisurely - Starts @ 90s and decrements by 5s per pick'
		roundTime = 90*time.Second - (5 * time.Second * (time.Duration(director.round) * time.Second))
		break
	case "slow":
		//'Slow - Starts @ 75s and decrements by 5s per pick'
		roundTime = 75*time.Second - (5 * time.Second * (time.Duration(director.round) * time.Second))
		break
	case "moderate":
		//'Moderate - Starts @ 55s A happy medium between slow, and fast.'
		roundTime = 55*time.Second - (5 * time.Second * (time.Duration(director.round) * time.Second))
		break
	case "fast":
		//'Fast - Starts @ 40s, based on official WOTC timing'
		roundTime = 40*time.Second - (5 * time.Second * (time.Duration(director.round) * time.Second))
		break
	}

	if roundTime < 15*time.Second {
		roundTime = 15 * time.Second
	}

	ticks := 0
	ticker := time.NewTicker(1 * time.Second)
	pickIncrease := make(chan int, len(director.Seats))
	logger := GetLogger()
	go func() {
		var picks = 0
		for {
			select {
			case <-ticker.C:
				ticks += 1
				if time.Duration(ticks)*time.Second == roundTime {
					logger.Infow("Times Up! Forcing autopicks and ending round", "round", director.round)
					go director.startNextRound()
					ticker.Stop()
					break
				}
				if picks == len(director.Seats) {
					logger.Infow("Everyone has picked, skipping to next round")
					go director.startNextRound()
					ticker.Stop()
					break
				}
				break
			case <-pickIncrease:
				picks += 1
			}
		}
	}()

	return pickIncrease
}

func (director *GameDirector) pause() {
	logger := GetLogger()
	logger.Infow("NO HOST! *PAUSING*.")
	ticks := 0
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ticker.C:
			ticks += 1
			if director.host != NO_HOST_SENTINEL {
				logger.Infow("NEW HOST! *UNPAUSING*.")
				ticker.Stop()
				break
			}
			if ticks == 6 {
				logger.Infow("Shutting down server. No host after 30 second grace period.")
				os.Exit(1)
			}
		}
	}
}

func (director *GameDirector) promoteNewHost() {
	nextHostId, err := getRandomClientId(director.Clients)
	if err != nil {
		director.host = NO_HOST_SENTINEL
	} else {
		director.host = nextHostId
		director.sendHostMessage(&Message{
			Type: HostChange,
			Data: strconv.Itoa(1),
		})
	}
}

type SetCard struct {
	Object        string `json:"object"`
	ID            string `json:"id"`
	OracleID      string `json:"oracle_id"`
	MultiverseIds []int  `json:"multiverse_ids"`
	MtgoID        int    `json:"mtgo_id"`
	ArenaID       int    `json:"arena_id"`
	TcgplayerID   int    `json:"tcgplayer_id"`
	Name          string `json:"name"`
	Lang          string `json:"lang"`
	ReleasedAt    string `json:"released_at"`
	URI           string `json:"uri"`
	ScryfallURI   string `json:"scryfall_uri"`
	Layout        string `json:"layout"`
	HighresImage  bool   `json:"highres_image"`
	ImageUris     struct {
		Small      string `json:"small"`
		Normal     string `json:"normal"`
		Large      string `json:"large"`
		Png        string `json:"png"`
		ArtCrop    string `json:"art_crop"`
		BorderCrop string `json:"border_crop"`
	} `json:"image_uris"`
	ManaCost      string   `json:"mana_cost"`
	Cmc           float64  `json:"cmc"`
	TypeLine      string   `json:"type_line"`
	OracleText    string   `json:"oracle_text"`
	Power         string   `json:"power"`
	Toughness     string   `json:"toughness"`
	Colors        []string `json:"colors"`
	ColorIdentity []string `json:"color_identity"`
	Legalities    struct {
		Standard  string `json:"standard"`
		Future    string `json:"future"`
		Historic  string `json:"historic"`
		Pioneer   string `json:"pioneer"`
		Modern    string `json:"modern"`
		Legacy    string `json:"legacy"`
		Pauper    string `json:"pauper"`
		Vintage   string `json:"vintage"`
		Penny     string `json:"penny"`
		Commander string `json:"commander"`
		Brawl     string `json:"brawl"`
		Duel      string `json:"duel"`
		Oldschool string `json:"oldschool"`
	} `json:"legalities"`
	Games           []string `json:"games"`
	Reserved        bool     `json:"reserved"`
	Foil            bool     `json:"foil"`
	Nonfoil         bool     `json:"nonfoil"`
	Oversized       bool     `json:"oversized"`
	Promo           bool     `json:"promo"`
	Reprint         bool     `json:"reprint"`
	Variation       bool     `json:"variation"`
	Set             string   `json:"set"`
	SetName         string   `json:"set_name"`
	SetType         string   `json:"set_type"`
	SetURI          string   `json:"set_uri"`
	SetSearchURI    string   `json:"set_search_uri"`
	ScryfallSetURI  string   `json:"scryfall_set_uri"`
	RulingsURI      string   `json:"rulings_uri"`
	PrintsSearchURI string   `json:"prints_search_uri"`
	CollectorNumber string   `json:"collector_number"`
	Digital         bool     `json:"digital"`
	Rarity          string   `json:"rarity"`
	FlavorText      string   `json:"flavor_text"`
	CardBackID      string   `json:"card_back_id"`
	Artist          string   `json:"artist"`
	ArtistIds       []string `json:"artist_ids"`
	IllustrationID  string   `json:"illustration_id"`
	BorderColor     string   `json:"border_color"`
	Frame           string   `json:"frame"`
	FullArt         bool     `json:"full_art"`
	Textless        bool     `json:"textless"`
	Booster         bool     `json:"booster"`
	StorySpotlight  bool     `json:"story_spotlight"`
	EdhrecRank      int      `json:"edhrec_rank"`
	Preview         struct {
		Source      string `json:"source"`
		SourceURI   string `json:"source_uri"`
		PreviewedAt string `json:"previewed_at"`
	} `json:"preview"`
	RelatedUris struct {
		Gatherer       string `json:"gatherer"`
		TcgplayerDecks string `json:"tcgplayer_decks"`
		Edhrec         string `json:"edhrec"`
		Mtgtop8        string `json:"mtgtop8"`
	} `json:"related_uris"`
}

type SetPacks struct {
	Packs [][]SetCard
}

func (director *GameDirector) getGameResources() {
	switch director.options.Type {
	case game.DRAFT:
		switch director.options.Mode {
		case game.CHAOS:
			opts := director.options.GameOptions.Draft.Chaos

			director.totalPacks = opts.TotalPacks
			break
		case game.CUBE:
			opts := director.options.GameOptions.Draft.Cube

			director.totalPacks = opts.TotalPacks
			break
		case game.REGULAR:
			opts := director.options.GameOptions.Draft.Regular
			for i := 0; i < opts.TotalPacks; i++ {
				setAbbrev := opts.SelectedPacks[strconv.Itoa(i)]
				res, err := http.Get(fmt.Sprintf("http://localhost:8000/set/%s/pack?n=%d", setAbbrev, director.options.TotalPlayers))
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

				playerPacks := make(map[int][]SetCard)
				for i, packs := range boosters.Packs {
					playerPacks[i] = packs
				}
				director.roundPacks[i] = DraftRound{
					SetAbbreviation: setAbbrev,
					PlayerPacks:     playerPacks,
				}
			}
			director.totalPacks = opts.TotalPacks
			break
		default:
			panic(fmt.Sprintf("Unknown game mode: %d", director.options.Mode))
		}
		break
	case game.SEALED:
		switch director.options.Mode {
		case game.CHAOS:
			opts := director.options.GameOptions.Sealed.Chaos

			director.totalPacks = opts.TotalPacks
			break
		case game.CUBE:
			//opts := director.options.GameOptions.Sealed.Cube

			director.totalPacks = 1
			break
		case game.REGULAR:
			opts := director.options.GameOptions.Sealed.Regular

			director.totalPacks = opts.TotalPacks
			break
		}
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
	logger := GetLogger()
	logger.Infow("Listening", "game", director.gameId, "port", director.port)
	// upgrade this connection to a WebSocket

	http.HandleFunc("/ws", director.newClient)
	logger.Infow("Created /ws handler")

	for {
		select {
		case c := <-director.addClientCh:
			logger.Debugw("Added new client")
			director.Clients[c.id] = c
			logger.Debugw("Total", "clients", len(director.Clients))
			go director.sendAll(&Message{
				Type: NewPlayer,
				Data: strconv.Itoa(len(director.Clients)),
			})
			go director.sendPastMessages(c)
		case c := <-director.delClientCh:

			clientID := c.id
			logger.Debugw("Removing client: ", clientID)
			delete(director.Clients, clientID)

			if len(director.Clients) == 0 {
				director.pause()
			} else {
				if clientID == director.host {
					director.promoteNewHost()
				}
				go director.SendAll(&Message{
					Type: NewPlayer,
					Data: strconv.Itoa(len(director.Clients)),
				})
			}
		case msg := <-director.sendAllCh:
			logger.Debugw("Sending to call clients", "msg", msg)
			director.messages = append(director.messages, msg)
			director.sendAll(msg)
		case err := <-director.errCh:
			logger.Error("Error", err.Error())
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
