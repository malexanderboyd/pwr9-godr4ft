package models

import "time"

const DraftCookieName = "pwr9_draft"
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
var (
	Newline = []byte{'\n'}
	Space   = []byte{' '}
)

const ChannelBufSize = 100

const (
	// Time allowed to write a message to the peer
	WriteWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer
	PongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait
	PingPeriod = (PongWait * 9) / 10

	// Maximum message size allowed from peer
	MaxMessageSize = 512
)