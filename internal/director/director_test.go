package director_test

import (
	"github.com/malexanderboyd/pwr9-godr4ft/internal/director"
	"github.com/malexanderboyd/pwr9-godr4ft/internal/game"
	"testing"
)

func TestCreateNewGameDirector(t *testing.T) {
	var mockOptions game.GeneralOptions

	var port = 9000
	var gameId = "a_test_game"


	d := director.NewGameDirector(mockOptions, port, gameId)

	if d.Port != port {
		t.Errorf("Director's port must match provided port=%d", port)
	}

	if d.GameId != gameId {
		t.Errorf("Directors gameId must match provided gameId=%s", gameId)
	}

}



func TestGameDirectorGetGameResources(t *testing.T) {

	var baseGeneralOptions = game.GeneralOptions{
		TotalPlayers: 2,
		PrivateGame: true,
		GameTitle: "test game",
		Mode: nil,
		Type: nil,
		GameOptions: nil,
	}


	var resourcestests = []struct {
		Type    game.Type
		Mode    game.Mode
		options game.GeneralOptions
	}{
		{game.DRAFT, game.REGULAR, baseGeneralOptions},
		{game.DRAFT, game.CUBE, baseGeneralOptions},
		{game.DRAFT, game.CHAOS, baseGeneralOptions},
	}
}
