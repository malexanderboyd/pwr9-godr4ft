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
	SelectedPacks map[string]string `json:"selectedPacks"`
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
	SelectedPacks	map[string]string `json:"selectedPacks"`
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
	Regular DraftRegularOptions `json:"1"`
	Cube DraftCubeOptions `json:"2"`
	Chaos DraftChaosOptions `json:"3"`
}

type SealedOptions struct {
	Regular SealedRegularOptions `json:"1"`
	Cube SealedCubeOptions `json:"2"`
	Chaos SealedChaosOptions `json:"3"`
}

type ModeMap struct {
	Draft  DraftOptions  `json:"1"`
	Sealed SealedOptions `json:"2"`
}

type GeneralOptions struct {
	TotalPlayers int `json:"totalPlayers"`
	GameTitle string `json:"gameTitle"`
	PrivateGame bool `json:"privateGame"`
	Mode Mode `json:"gameMode"`
	Type Type `json:"gameType"`
	GameOptions ModeMap `json:"options"`
}