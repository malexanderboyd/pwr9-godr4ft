package game

type Type int

const (
	DRAFT  Type = 1
	SEALED Type = 2
)

type Mode int

const (
	REGULAR Mode = 1
	CUBE    Mode = 2
	CHAOS   Mode = 3
)

type DraftRegularOptions struct {
	TotalPacks    int            `json:"totalPacks"`
	SelectedPacks map[int]string `json:"selectedPacks"`
}

type DraftCubeOptions struct {
	CardsPerPack	int	`json:"cardsPerPack"`
	TotalPacks		int	`json:"totalPacks"`
	CubeList		string `json:"cubeList"`
}

type DraftChaosOptions struct {
	TotalPacks	int `json:"totalPacks"`
	OnlyModern	bool	`json:"onlyModern"`
	TotalChaos	bool	`json:"totalChaos"`
}

type SealedRegularOptions struct {
	TotalPacks	int `json:"totalPacks"`
	SelectedPacks	map[int]string `json:"selectedPacks"`
}

type SealedCubeOptions struct {
	CardsPerPlayer	int `json:"cardsPerPlayer"`
	CubeList	string	`json:"cubeList"`
}

type SealedChaosOptions struct {
	TotalPacks int `json:"totalPacks"`
	OnlyModern bool	`json:"onlyModern"`
	TotalChaos bool `json:"totalChaos"`
}

type DraftOptions struct {
	Regular DraftRegularOptions
	Cube DraftCubeOptions
	Chaos DraftChaosOptions
}

type SealedOptions struct {
	Regular SealedRegularOptions
	Cube SealedCubeOptions
	Chaos SealedChaosOptions
}

type ModeMap struct {
	draft map[Mode]DraftOptions
	sealed map[Mode]SealedOptions
}

type GeneralOptions struct {
	TotalPlayers int `json:"totalPlayers"`
	GameTitle string `json:"gameTitle"`
	PrivateGame bool `json:"privateGame"`
	Mode Mode `json:"gameMode"`
	Type Type `json:"gameType"`
	GameOptions map[Type]ModeMap `json:"gameOptions"`
}