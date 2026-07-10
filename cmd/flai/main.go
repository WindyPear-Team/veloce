package main

import (
	"log"

	"github.com/WindyPear-Team/veloce/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
