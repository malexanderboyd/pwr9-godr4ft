package director

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/malexanderboyd/pwr9-godr4ft/internal"
	"github.com/malexanderboyd/pwr9-godr4ft/internal/director/models"
	"github.com/malexanderboyd/pwr9-godr4ft/internal/director/utils"
	"github.com/malexanderboyd/pwr9-godr4ft/internal/game"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

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
	roundTimerServerForcePick bool
	roundPacks         map[int]models.DraftRound
	roundPicksTickerCh chan int
	nextRoundPacks     map[int][]models.SetCard
	totalPacks         int
	host               string
	Clients            map[string]*Client
	Seats              map[string]int
	messages           []*models.Message
	addClientCh        chan *Client
	delClientCh        chan *Client
	sendAllCh          chan *models.Message
	startNextRoundCh   chan bool
	doneCh             chan bool
	errCh              chan error
}

func NewGameDirector(options game.GeneralOptions, port int, gameId string) *GameDirector {
	return &GameDirector{
		clientsContents:    nil,
		roundPacks:         make(map[int]models.DraftRound),
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
		nextRoundPacks:     make(map[int][]models.SetCard),
		totalPacks:         0,
		host:               models.NoHostSentinel,
		Clients:            make(map[string]*Client),
		messages:           []*models.Message{},
		addClientCh:        make(chan *Client),
		delClientCh:        make(chan *Client),
		sendAllCh:          make(chan *models.Message),
		startNextRoundCh:   make(chan bool),
		doneCh:             make(chan bool),
		errCh:              make(chan error),
	}
}

func (director *GameDirector) AddNewClient(c *Client) {
	director.addClientCh <- c
}

func (director *GameDirector) DeleteClient(c *Client) {
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

func (director *GameDirector) SendAll(msg *models.Message) {
	director.sendAllCh <- msg
}

func (director *GameDirector) sendAll(msg *models.Message) {
	for _, c := range director.Clients {
		c.Write(msg)
	}
}

func (director *GameDirector) sendHostMessage(msg *models.Message) {
	host := director.Clients[director.host]
	if host != nil {
		host.Write(msg)
	}
}

func (director *GameDirector) newClient(w http.ResponseWriter, r *http.Request) {
	//var hasCookie, clientID = utils.HasDraftClientIDCookie(r, models.DraftCookieName)
	//if hasCookie && director.isExistingClien t(clientID) {
	//	director.Clients[clientID].Done()
	//}
	var err error
	newClient, err := NewClient(director)
	if err != nil {
		director.Error(err)
		return
	}

	DraftClientIDCookieHeader := utils.CreateDraftClientIDCookieHeader(newClient.Id, models.DraftCookieName)

	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	ws, err := upgrader.Upgrade(w, r, DraftClientIDCookieHeader)
	if err != nil {
		director.Error(err)
		_, _ = fmt.Fprintf(w, err.Error())
	}

	newClient.Websocket = ws

	if director.host == models.NoHostSentinel {
		director.host = newClient.Id
		newClient.Write(&models.Message{
			Type: models.HostChange,
			Data: strconv.Itoa(1),
		})
	}
	director.AddNewClient(newClient)
	go newClient.Listen()
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

func (director *GameDirector) HandleClientMessage(clientID string, msg *models.Message) {
	logger := internal.GetLogger()
	switch msg.Type {
	case models.ChatMessage:
		director.SendAll(msg)
		break
	case models.GameStart:
		if !director.gameStarted {
			var timerSetting = &models.TimerSettings{}
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
	case models.ChooseCard:
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

func (director *GameDirector) getPackByClientID(clientId string) []models.SetCard {
	playerSeat := director.getSeatByClientId(clientId)
	return director.roundPacks[director.packNumber].PlayerPacks[playerSeat]
}

func (director *GameDirector) handleClientChooseCard(clientID string, msg *models.Message) error {
	rawMsgContents := msg.Data
	client := director.Clients[clientID]
	if client == nil {
		return errors.New(fmt.Sprintf("No client with id: %s. Must provide valid client ID", clientID))
	} else {

		var selectedCardMsg models.ChooseCardJson
		if err := json.Unmarshal([]byte(rawMsgContents), &selectedCardMsg); err != nil {
			director.Error(err)
		}

		currentPack := director.getPackByClientID(client.Id)
		if currentPack == nil {
			return errors.New(fmt.Sprintf("client %s already chose this round, resent chose_card msg", client.Id))
		}

		if selectedCardMsg.PickedCardIndex >= len(currentPack) || selectedCardMsg.PickedCardIndex < 0 {
			return errors.New(fmt.Sprintf("[client %s] chose an invalid card index %d", clientID, selectedCardMsg.PickedCardIndex))
		}

		chosenCard := currentPack[selectedCardMsg.PickedCardIndex]

		currentPack = append(currentPack[:selectedCardMsg.PickedCardIndex], currentPack[selectedCardMsg.PickedCardIndex+1:]...)

		playerSeat := director.getSeatByClientId(client.Id)
		nextClientSeat := director.getSeatNumberForNextRound(playerSeat)
		director.nextRoundPacks[nextClientSeat] = currentPack
		director.roundPacks[director.packNumber].PlayerPacks[playerSeat] = nil
		client.AddCardToPool(chosenCard)
		client.WriteCurrentPool()
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

func (director *GameDirector) getSeatNumberForNextRound(currentSeat int) int {
	if director.packNumber%2 == 0 {
		// rounds go left, right, left ...
		if currentSeat+1 >= len(director.Seats) {
			return 0
		} else {
			return currentSeat + 1
		}
	} else {
		if currentSeat-1 < 0 {
			return len(director.Seats) - 1
		} else {
			return currentSeat - 1
		}
	}
}

func (director *GameDirector) pickCardsForStallingClients() {
	for seatNum, pp := range director.roundPacks[director.packNumber].PlayerPacks {
		if pp != nil {

			forcePick, err := json.Marshal(&models.ChooseCardJson{
				PickedCardIndex: 0,
			})
			if err != nil {
				director.Error(err)
				director.shutdown()
				break
			}

			clientId := director.getClientIdBySeat(seatNum)
			err = director.handleClientChooseCard(clientId, &models.Message{
				Type: models.ChooseCard,
				Data: string(forcePick),
			})

			if err != nil {
				director.Error(err)
				director.shutdown()
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
			CurrentRound := director.roundPacks[director.packNumber]
			var currentPlayer = 0
			for clientID, client := range director.Clients {

				if currentPlayer >= len(CurrentRound.PlayerPacks) {
					break
				}

				playerPack := CurrentRound.PlayerPacks[currentPlayer]
				director.Seats[clientID] = currentPlayer


				newPack := &models.CardPack{
					SetName:    CurrentRound.SetAbbreviation,
					Pack:       playerPack,
					Round:      director.round,
					PackNumber: director.packNumber + 1,
				}

				if director.isTimerEnabled() {
					newPack.Timer = int(director.getRoundTimer() / time.Second)
				}


				emp, _ := json.Marshal(newPack)

				go client.Write(&models.Message{
					Type: models.RoundContent,
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
	logger := internal.GetLogger()
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

		newPack := &models.CardPack{
			SetName:    CurrentPackRound.SetAbbreviation,
			Pack:       playerPack,
			Round:      director.round,
			PackNumber: director.packNumber + 1,
		}

		if director.isTimerEnabled() {
			newPack.Timer = int(director.getRoundTimer() / time.Second)
		}

		emp, _ := json.Marshal(newPack)


		client.Write(&models.Message{
			Type: models.RoundContent,
			Data: string(emp),
		})

	}
	director.roundPicksTickerCh = director.startRoundPicksTicker()
}

func (director *GameDirector) isTimerEnabled() bool {
	return director.roundTimerType != ""
}

func (director *GameDirector) isServerForcePickEnabled() bool {
	return director.roundTimerServerForcePick == true
}

func (director *GameDirector) getRoundTimer() time.Duration {
	var roundTime = 1 * time.Second
	switch director.roundTimerType {
	case "leisurely":
		//'Leisurely - Starts @ 90s and decrements by 5s per pick'
		roundTime = 90*time.Second - (5 * time.Second * (time.Duration(director.round - 1)))
		break
	case "slow":
		//'Slow - Starts @ 75s and decrements by 5s per pick'
		roundTime = 75*time.Second - (5 * time.Second * (time.Duration(director.round - 1)))
		break
	case "moderate":
		//'Moderate - Starts @ 55s A happy medium between slow, and fast.'
		roundTime = 55*time.Second - (5 * time.Second * (time.Duration(director.round - 1)))
		break
	case "fast":
		//'Fast - Starts @ 40s, based on official WOTC timing'
		roundTime = 40*time.Second - (5 * time.Second * (time.Duration(director.round - 1)))
		break
	}

	if roundTime < 3*time.Second {
		roundTime = 3 * time.Second
	}
	return roundTime
}

func (director *GameDirector) startRoundPicksTicker() chan int {

	logger := internal.GetLogger()
	var roundTime time.Duration
	if director.isTimerEnabled() {
		roundTime = director.getRoundTimer()
	}

	ticks := 0
	ticker := time.NewTicker(1 * time.Second)
	pickIncrease := make(chan int, len(director.Seats))
	go func() {
		var picks = 0
		for {
			select {
			case <-ticker.C:
				ticks += 1
				if director.isTimerEnabled() && director.isServerForcePickEnabled() && time.Duration(ticks)*time.Second == roundTime {
					logger.Infow("Times Up! Forcing autopicks and ending round", "round", director.round)
					director.startNextRoundCh <- true
					close(pickIncrease)
					ticker.Stop()
					return
				} else if picks == len(director.Seats) {
					logger.Infow("all players have picked, ending round", "round", director.round)
					director.startNextRoundCh <- true
					close(pickIncrease)
					ticker.Stop()
					return
				}
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
	logger := internal.GetLogger()
	logger.Infow("NO HOST! *PAUSING*.")
	ticks := 0
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ticker.C:
			ticks += 1
			if director.host != models.NoHostSentinel {
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
	var nextHostId string
	for k := range director.Clients {
		nextHostId = k
	}
	if nextHostId == "" {
		director.host = models.NoHostSentinel
	} else {
		director.host = nextHostId
		director.sendHostMessage(&models.Message{
			Type: models.HostChange,
			Data: strconv.Itoa(1),
		})
	}
}

func (director *GameDirector) getGameResources() error {
	logger := internal.GetLogger()
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
				var boosters models.SetPacks
				msg, err := ioutil.ReadAll(res.Body)
				if err != nil {
					panic(err)
				}

				if err := json.Unmarshal(msg, &boosters); err != nil {
					logger.Fatalw("cannot decode booster json for packs", "set", setAbbrev)
					panic(err)
				}

				playerPacks := make(map[int][]models.SetCard)
				for i, packs := range boosters.Packs {
					playerPacks[i] = packs
				}
				director.roundPacks[i] = models.DraftRound{
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
	logger := internal.GetLogger()
	logger.Infow("Listening", "game", director.GameId, "port", director.Port)
	// upgrade this connection to a WebSocket

	http.HandleFunc("/ws", director.newClient)
	logger.Infow("Created /ws handler")

	for {
		select {
		case c := <-director.addClientCh:
			logger.Debugw("Added new client")
			director.Clients[c.Id] = c
			logger.Debugw("Total", "clients", len(director.Clients))
			go director.sendAll(&models.Message{
				Type: models.NewPlayer,
				Data: strconv.Itoa(len(director.Clients)),
			})
			go director.sendPastMessages(c)
		case c := <-director.delClientCh:
			clientID := c.Id
			logger.Debugw("Removing client", "client", clientID)
			delete(director.Clients, clientID)

			if clientID == director.host {
				director.promoteNewHost()
			}
			go director.SendAll(&models.Message{
				Type: models.NewPlayer,
				Data: strconv.Itoa(len(director.Clients)),
			})
		case msg := <-director.sendAllCh:
			if msg.Type != models.RoundContent {
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
			logger.Errorw("error occurred", "error", err.Error())
		case <-director.doneCh:
			director.sendAll(&models.Message{
				Type: models.GameEnd,
				Data: strconv.Itoa(len(director.Clients)),
			})
			for _, c := range director.Clients {
				c.Done()
			}
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
		return "http://localhost/api"
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
