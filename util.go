package main

import (
	"errors"
	"net/http"
	"time"
)

// generated from https://mholt.github.io/json-to-go/
type SetCard struct {
	Artist                string        `json:"artist"`
	BorderColor           string        `json:"borderColor"`
	ColorIdentity         []string      `json:"colorIdentity"`
	Colors                []string      `json:"colors"`
	ConvertedManaCost     float64       `json:"convertedManaCost"`
	EdhrecRank            int           `json:"edhrecRank"`
	FaceConvertedManaCost float64       `json:"faceConvertedManaCost"`
	FlavorText            string        `json:"flavorText"`
	ForeignData           []interface{} `json:"foreignData"`
	FrameEffect           string        `json:"frameEffect"`
	FrameEffects          []string      `json:"frameEffects"`
	FrameVersion          string        `json:"frameVersion"`
	HasFoil               bool          `json:"hasFoil"`
	HasNonFoil            bool          `json:"hasNonFoil"`
	IsMtgo                bool          `json:"isMtgo"`
	IsPaper               bool          `json:"isPaper"`
	IsPromo               bool          `json:"isPromo"`
	IsStarter             bool          `json:"isStarter"`
	Layout                string        `json:"layout"`
	Legalities            struct {
		Brawl     string `json:"brawl"`
		Commander string `json:"commander"`
		Duel      string `json:"duel"`
		Future    string `json:"future"`
		Historic  string `json:"historic"`
		Legacy    string `json:"legacy"`
		Modern    string `json:"modern"`
		Penny     string `json:"penny"`
		Pioneer   string `json:"pioneer"`
		Standard  string `json:"standard"`
		Vintage   string `json:"vintage"`
	} `json:"legalities"`
	ManaCost     string   `json:"manaCost"`
	Name         string   `json:"name"`
	Names        []string `json:"names"`
	Number       string   `json:"number"`
	OtherFaceIds []string `json:"otherFaceIds"`
	Prices       struct {
		Mtgo struct {
		} `json:"mtgo"`
		MtgoFoil struct {
		} `json:"mtgoFoil"`
		Paper struct {
		} `json:"paper"`
		PaperFoil struct {
		} `json:"paperFoil"`
	} `json:"prices"`
	Printings    []string `json:"printings"`
	PurchaseUrls struct {
		Tcgplayer string `json:"tcgplayer"`
	} `json:"purchaseUrls"`
	Rarity  string `json:"rarity"`
	Rulings []struct {
		Date string `json:"date"`
		Text string `json:"text"`
	} `json:"rulings"`
	ScryfallID             string        `json:"scryfallId"`
	ScryfallIllustrationID string        `json:"scryfallIllustrationId"`
	ScryfallOracleID       string        `json:"scryfallOracleId"`
	Side                   string        `json:"side"`
	Subtypes               []string      `json:"subtypes"`
	Supertypes             []interface{} `json:"supertypes"`
	TcgplayerProductID     int           `json:"tcgplayerProductId"`
	Text                   string        `json:"text"`
	Type                   string        `json:"type"`
	Types                  []string      `json:"types"`
	UUID                   string        `json:"uuid"`
	Variations             []string      `json:"variations"`
}

type SetPacks struct {
	Packs [][]SetCard
}

func getRandomClientId(m map[string]*Client) (string, error) {
	for k := range m {
		return k, nil
	}
	return "", errors.New("no clients available")
}

func createDraftClientIDCookie(clientID string) http.Header {
	var clientIDHeader = http.Header{}
	clientIdCookie := &http.Cookie{
		Name:  DraftCookieName,
		Value: clientID,
		Path:  "/",
		Expires: time.Now().Add(time.Minute * 30),
	}
	if v := clientIdCookie.String(); v != "" {
		clientIDHeader.Add("Set-Cookie", v)
	}
	return clientIDHeader
}

func hasDraftClientIDCookie(r *http.Request) (bool, string) {
	cookies := r.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == DraftCookieName {
			return true, cookie.Value
		}
	}
	return false, ""
}