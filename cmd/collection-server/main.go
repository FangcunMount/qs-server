package main

import (
	"math/rand"
	"time"

	collection "github.com/fangcun-mount/qs-server/internal/collection-server"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	command := collection.NewApp("collection-server")
	command.Run()
}
