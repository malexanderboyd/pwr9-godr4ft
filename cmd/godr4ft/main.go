package main

import (
	"flag"
	"github.com/malexanderboyd/pwr9-godr4ft/internal/director"
	"log"
)


func main() {
	log.SetFlags(log.Lshortfile)

	port := flag.Int("port", 8000, "the port the server will open a socket server on")
	gameId := flag.String("gameId", "", "Four byte url safe hex string")
	flag.Parse()

	director.StartDraftServer(*gameId, *port)
}



