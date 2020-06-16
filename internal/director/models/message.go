package models

type Message struct {
	Type GameMessageType `json:"type"`
	Data string          `json:"data"`
}
