package main

import (
	"math/rand"
	"time"

	collection "github.com/FangcunMount/qs-server/internal/collection-server"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	command := collection.NewApp("collection-server")
	command.Run()
}
