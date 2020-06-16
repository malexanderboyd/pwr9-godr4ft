package models

type TimerSettings struct {
	Type string `json:"timer"`
	ForcePick bool `json:"serverForcePick"`
}
