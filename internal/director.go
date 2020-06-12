package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/malexanderboyd/pwr9-godr4ft/internal/game"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type GameMessageType string

const NoHostSentinel = "-999"
const (
	NewPlayer    GameMessageType = "new_player"
	ChatMessage  GameMessageType = "chat_message"
	HostChange   GameMessageType = "host_change"
	GameStart    GameMessageType = "start_game"
	GameEnd      GameMessageType = "end_game"
	RoundContent GameMessageType = "round_content"
	PoolContent  GameMessageType = "pool_content"
	ChooseCard   GameMessageType = "choose_card"
)

type CardPack struct {
	SetName    string    `json:"setName"`
	Round      int       `json:"round"`
	PackNumber int       `json:"packNumber"`
	Pack       []SetCard `json:"pack"`
	Timer      int       `json:"timer"`
}

const DraftCookieName = "pwr9_draft"

type Message struct {
	Type GameMessageType `json:"type"`
	Data string          `json:"data"`
}

type DraftRound struct {
	SetAbbreviation string
	PlayerPacks     map[int][]SetCard
}

type DraftPool struct {
	Cards []SetCard `json:"cards"`
}

type ChooseCardJson struct {
	PickedCardIndex int `json:"pickedCardIndex"`
}

func (dr *DraftRound) getPlayerPacksBySeat(playerSeatNumber int) []SetCard {
	return dr.PlayerPacks[playerSeatNumber]
}

type GameDirector struct {
	clientsContents map[string][]string
	pool            []string
	//
	Port               int
	GameId             string
	options            game.GeneralOptions
	gameStarted        bool
	packNumber         int
	round              int
	roundTimerType     string
	roundPacks         map[int]DraftRound
	roundPicksTickerCh chan int
	nextRoundPacks     map[int][]SetCard
	totalPacks         int
	host               string
	Clients            map[string]*Client
	Seats              map[string]int
	messages           []*Message
	addClientCh        chan *Client
	delClientCh        chan *Client
	sendAllCh          chan *Message
	startNextRoundCh   chan bool
	doneCh            chan bool
	errCh             chan error
}

func NewGameDirector(options game.GeneralOptions, port int, gameId string) *GameDirector {
	return &GameDirector{
		clientsContents:    nil,
		roundPacks:         make(map[int]DraftRound),
		pool:               nil,
		Port:               port,
		GameId:             gameId,
		options:            options,
		gameStarted:        false,
		packNumber:         0,
		roundTimerType:     "",
		round:              1,
		roundPicksTickerCh: nil,
		Seats:              make(map[string]int),
		nextRoundPacks:     make(map[int][]SetCard),
		totalPacks:         0,
		host:               NoHostSentinel,
		Clients:            make(map[string]*Client),
		messages:           []*Message{},
		addClientCh:        make(chan *Client),
		delClientCh:        make(chan *Client),
		sendAllCh:          make(chan *Message),
		startNextRoundCh:   make(chan bool),
		doneCh:             make(chan bool),
		errCh:              make(chan error),
	}
}

func (director *GameDirector) addNewClient(c *Client) {
	director.addClientCh <- c
}

func (director *GameDirector) deleteClient(c *Client) {
	director.delClientCh <- c
}

func (director *GameDirector) shutdown() {
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
	logger := GetLogger()
	var client *Client
	var oldMessages []*Message
	var hasCookie, clientID = hasDraftClientIDCookie(r)
	if hasCookie && director.isExistingClient(clientID) {
		client = director.Clients[clientID]
		oldMessages = client.messages
		client.doneCh <- true
		logger.Debugw("reconnecting", "client", client.id)
	} else {
		var err error
		client, err = NewClient(director)
		if err != nil {
			director.Error(err)
			return
		}
	}

	createDraftClientIDCookie := createDraftClientIDCookie(client.id)
	ws, err := upgrader.Upgrade(w, r, createDraftClientIDCookie)
	if err != nil {
		director.Error(err)
		_, _ = fmt.Fprintf(w, err.Error())
	}
	client.ws = ws
	if client.messages != nil {
		client.messages = oldMessages
	}

	if director.host == NoHostSentinel {
		director.host = client.id
		client.Write(&Message{
			Type: HostChange,
			Data: strconv.Itoa(1),
		})
	}
	director.addNewClient(client)
	go client.Listen()
}

func (director *GameDirector) isExistingClient(clientId string) bool {
	if clientId == "" {
		return false
	}

	if _, ok := director.Clients[clientId]; !ok {
		return false
	}
	return true
}

func (director *GameDirector) handleClientMessage(clientID string, msg *Message) {
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
			} else {
				director.roundTimerType = timerSetting.Type
			}
			logger.Infow("Starting Game!")
			director.SendAll(msg)
			go director.startGame()
		}
		break
	case ChooseCard:
		if director.gameStarted {
			if err := director.handleClientChooseCard(clientID, msg); err != nil {
				director.Error(err)
			} else {
				director.roundPicksTickerCh <- 1
			}
		}
		break
	default:
		break
	}
}

func (director *GameDirector) getSeatByClientId(clientId string) int {
	return director.Seats[clientId]
}

func (director *GameDirector) getClientIdBySeat(target int) string {
	for id, seatNumber := range director.Seats {
		if seatNumber == target {
			return id
		}
	}
	return ""
}

func (director *GameDirector) getPackByClientID(clientId string) []SetCard {
	playerSeat := director.getSeatByClientId(clientId)
	return director.roundPacks[director.packNumber].PlayerPacks[playerSeat]
}

func (director *GameDirector) handleClientChooseCard(clientID string, msg *Message) error {
	rawMsgContents := msg.Data
	client := director.Clients[clientID]
	if client == nil {
		return errors.New(fmt.Sprintf("No client with id: %s. Must provide valid client ID", clientID))
	} else {

		var selectedCardMsg ChooseCardJson
		if err := json.Unmarshal([]byte(rawMsgContents), &selectedCardMsg); err != nil {
			director.Error(err)
		}

		currentPack := director.getPackByClientID(client.id)
		if currentPack == nil {
			return errors.New(fmt.Sprintf("client %s already chose this round, resent chose_card msg", client.id))
		}

		if selectedCardMsg.PickedCardIndex >= len(currentPack) || selectedCardMsg.PickedCardIndex < 0 {
			return errors.New(fmt.Sprintf("[client %s] chose an invalid card index %d", clientID, selectedCardMsg.PickedCardIndex))
		}

		chosenCard := currentPack[selectedCardMsg.PickedCardIndex]
		client.pool = append(client.pool, chosenCard)

		currentPack = append(currentPack[:selectedCardMsg.PickedCardIndex], currentPack[selectedCardMsg.PickedCardIndex+1:]...)

		playerSeat := director.getSeatByClientId(client.id)
		var nextClientSeat int
		if director.packNumber%2 == 0 {
			// rounds go left, right, left ...
			if playerSeat+1 >= len(director.Seats) {
				nextClientSeat = 0
			} else {
				nextClientSeat = playerSeat + 1
			}
		} else {
			if playerSeat-1 < 0 {
				nextClientSeat = len(director.Seats) - 1
			} else {
				nextClientSeat = playerSeat - 1
			}
		}
		director.nextRoundPacks[nextClientSeat] = currentPack
		director.roundPacks[director.packNumber].PlayerPacks[playerSeat] = nil
		director.sendClientPool(client)
	}
	return nil
}

func (director *GameDirector) haveAllClientsPickedCurrentRound() bool {
	for _, pp := range director.roundPacks[director.packNumber].PlayerPacks {
		if pp != nil {
			return false
		}
	}
	return true
}

func (director *GameDirector) pickCardsForStallingClients() {
	for seatNum, pp := range director.roundPacks[director.packNumber].PlayerPacks {
		if pp != nil {

			forcePick, err := json.Marshal(&ChooseCardJson{
				PickedCardIndex: 0,
			})
			if err != nil {
				director.Error(err)
				director.shutdown()
				break
			}

			clientId := director.getClientIdBySeat(seatNum)
			err = director.handleClientChooseCard(clientId, &Message{
				Type: ChooseCard,
				Data: string(forcePick),
			})

			if err != nil {
				director.Error(err)
				director.shutdown()
			}
		}
	}
}

func (director *GameDirector) sendClientPool(client *Client) {
	jsonData, err := json.Marshal(client.pool)
	if err != nil {
		director.Error(err)
	} else {
		client.Write(&Message{
			Type: PoolContent,
			Data: string(jsonData),
		})
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
			CurrentRound := director.roundPacks[director.packNumber]
			var currentPlayer = 0
			for clientID, client := range director.Clients {

				if currentPlayer >= len(CurrentRound.PlayerPacks) {
					break
				}

				playerPack := CurrentRound.PlayerPacks[currentPlayer]
				director.Seats[clientID] = currentPlayer

				emp, _ := json.Marshal(&CardPack{
					SetName:    CurrentRound.SetAbbreviation,
					Pack:       playerPack,
					Round:      director.round,
					PackNumber: director.packNumber + 1,
					Timer:      int(director.getRoundTimer() / time.Second),
				})

				go client.Write(&Message{
					Type: RoundContent,
					Data: string(emp),
				})

				currentPlayer++
			}
			break
		default:
			panic(fmt.Sprintf("Unknown game mode: %d", director.options.Mode))
		}
		director.roundPicksTickerCh = director.startRoundPicksTicker()
		break
	case game.SEALED:
		break
	default:
		panic(fmt.Sprintf("Unknown game type: %d", director.options.Type))
	}
}

func (director *GameDirector) IsEndOfDraft() bool {
	if _, ok := director.roundPacks[director.packNumber]; !ok {
		return true
	}
	return false
}

func (director *GameDirector) startNextPack() {
	director.packNumber += 1
	logger := GetLogger()
	logger.Infow("Starting next pack", "pack_number", director.packNumber)
}

func (director *GameDirector) startNextRound() {
	CurrentPackRound := director.roundPacks[director.packNumber]
	for clientID, i := range director.Seats {

		if i >= len(CurrentPackRound.PlayerPacks) {
			break
		}
		client := director.Clients[clientID]
		playerPack := CurrentPackRound.PlayerPacks[i]

		emp, _ := json.Marshal(&CardPack{
			SetName:    CurrentPackRound.SetAbbreviation,
			Pack:       playerPack,
			Round:      director.round,
			PackNumber: director.packNumber + 1,
			Timer:      int(director.getRoundTimer()),
		})

		client.Write(&Message{
			Type: RoundContent,
			Data: string(emp),
		})

	}
	director.roundPicksTickerCh = director.startRoundPicksTicker()
}

func (director *GameDirector) getRoundTimer() time.Duration {
	var roundTime = 1 * time.Second
	switch director.roundTimerType {
	case "leisurely":
		//'Leisurely - Starts @ 90s and decrements by 5s per pick'
		roundTime = 90*time.Second - (5 * time.Second * (time.Duration(director.round-1)))
		break
	case "slow":
		//'Slow - Starts @ 75s and decrements by 5s per pick'
		roundTime = 75*time.Second - (5 * time.Second * (time.Duration(director.round-1)))
		break
	case "moderate":
		//'Moderate - Starts @ 55s A happy medium between slow, and fast.'
		roundTime = 55*time.Second - (5 * time.Second * (time.Duration(director.round-1)))
		break
	case "fast":
		//'Fast - Starts @ 40s, based on official WOTC timing'
		roundTime = 40*time.Second - (5 * time.Second * (time.Duration(director.round-1)))
		break
	}

	if roundTime < 15*time.Second {
		roundTime = 15 * time.Second
	}
	return roundTime
}

func (director *GameDirector) startRoundPicksTicker() chan int {

	logger := GetLogger()
	roundTime := director.getRoundTimer()
	ticks := 0
	ticker := time.NewTicker(1 * time.Second)
	pickIncrease := make(chan int, len(director.Seats))
	go func() {
		var picks = 0
		for {
			select {
			case <-ticker.C:
				ticks += 1
				if time.Duration(ticks)*time.Second == roundTime {
					logger.Infow("Times Up! Forcing autopicks and ending round", "round", director.round)
					director.startNextRoundCh <- true
					close(pickIncrease)
					ticker.Stop()
					break
				} else if picks == len(director.Seats) {
					logger.Infow("all players have picked, ending round", "round", director.round)
					director.startNextRoundCh <- true
					close(pickIncrease)
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

func (director *GameDirector) rotateCards() {
	for key, pack := range director.nextRoundPacks {
		director.roundPacks[director.packNumber].PlayerPacks[key] = pack
	}
}

func (director *GameDirector) shouldStartNewPack() bool {
	var totalEmptyPacks = 0
	for _, pack := range director.nextRoundPacks {
		if len(pack) == 0 {
			totalEmptyPacks += 1
		}
	}
	if totalEmptyPacks == len(director.Seats) {
		return true
	}
	return false
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
			if director.host != NoHostSentinel {
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
		director.host = NoHostSentinel
	} else {
		director.host = nextHostId
		director.sendHostMessage(&Message{
			Type: HostChange,
			Data: strconv.Itoa(1),
		})
	}
}

func (director *GameDirector) getGameResources() error {
	logger := GetLogger()
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
				res, err := http.Get(fmt.Sprintf("%s/set/%s/pack?n=%d", ApiUri, setAbbrev, director.options.TotalPlayers))
				if err != nil {
					logger.Fatalw("cannot get game options", "error", err.Error())
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
			return errors.New(fmt.Sprintf("Unknown game mode: %d", director.options.Mode))
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
		return errors.New(fmt.Sprintf("Unknown game mode: %d", director.options.Mode))
	}
	return nil
}


func (director *GameDirector) Listen() {
	logger := GetLogger()
	logger.Infow("Listening", "game", director.GameId, "port", director.Port)
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
			go c.sendOldMessages()
		case c := <-director.delClientCh:

			clientID := c.id
			logger.Debugw("Removing client", "client", clientID)
			delete(director.Clients, clientID)

			if len(director.Clients) == 0 {
				// director.pause()
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
			if msg.Type != RoundContent {
				logger.Debugw("Sending to all clients", "msg", msg)
			}
			director.messages = append(director.messages, msg)
			director.sendAll(msg)
		case <-director.startNextRoundCh:
			if director.shouldStartNewPack() {
				director.startNextPack()
				director.round = 1
			} else {
				if !director.haveAllClientsPickedCurrentRound() {
					director.pickCardsForStallingClients()
				}
				director.round += 1
				director.rotateCards()
			}
			if director.IsEndOfDraft() {
				logger.Infow("shutting down")
				go director.shutdown()
			} else {
				director.startNextRound()
			}
		case err := <-director.errCh:
			logger.Errorw("error occurred while sending messages to all clients", "error", err.Error())
		case <-director.doneCh:
			director.sendAll(&Message{
				Type: GameEnd,
				Data: strconv.Itoa(len(director.Clients)),
			})
			logger.Infow("Ended Game.", "game", director.GameId)
			os.Exit(0)
		}
	}

}

func getGeneralGameOptions(Url string) (game.GeneralOptions, error) {
	var gameOptions game.GeneralOptions
	res, err := http.Get(Url)
	defer res.Body.Close()
	if err != nil {
		return gameOptions, err
	} else {
		if err := json.NewDecoder(res.Body).Decode(&gameOptions); err != nil {
			return gameOptions, err
		} else {
			return gameOptions, nil
		}
	}
}

func getAPIUrlFromEnv(envKey string) string {
	if envKey == "" {
		envKey = "NODE_ENV"
	}
	ENV := os.Getenv(envKey)
	if ENV == "docker" {
		return "http://api:8002"
	} else {
		return  "http://localhost/api"
	}
}


var ApiUri string

func StartDraftServer(gameId string, port int) {
	ApiUri = getAPIUrlFromEnv("NODE_ENV")
	gameOptions, err := getGeneralGameOptions(fmt.Sprintf("%s/game/%s", ApiUri, gameId))
	if err != nil {
		panic(err)
	}

	director := NewGameDirector(gameOptions, port, gameId)

	if err := director.getGameResources(); err != nil {
		panic(err)
	}

	go director.Listen()

	http.Handle("/", http.FileServer(http.Dir("webroot")))
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
