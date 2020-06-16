package models

type CardPack struct {
	SetName    string          `json:"setName"`
	Round      int             `json:"round"`
	PackNumber int             `json:"packNumber"`
	Pack       []SetCard `json:"pack"`
	Timer      int             `json:"timer"`
}
