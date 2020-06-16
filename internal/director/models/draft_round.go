package models

type DraftRound struct {
	SetAbbreviation string
	PlayerPacks     map[int][]SetCard
}

func (dr *DraftRound) getPlayerPacksBySeat(playerSeatNumber int) []SetCard {
	return dr.PlayerPacks[playerSeatNumber]
}
