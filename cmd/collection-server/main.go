package main

import (
	"math/rand"
	"os"
	"time"

	collection "github.com/yshujie/questionnaire-scale/internal/collection-server"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	if len(os.Args) != 1 {
		os.Args = []string{os.Args[0]}
	}

	command := collection.NewApp("collection-server")
	command.Run()
}
