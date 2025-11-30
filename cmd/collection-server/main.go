package main

import (
	"math/rand"
	"time"

	collection "github.com/FangcunMount/qs-server/internal/collection-server"
	_ "github.com/FangcunMount/qs-server/internal/collection-server/docs"
)

// @title Collection Server
// @version 1.0
// @description Questionnaire collection/BFF layer
// @BasePath /api/v1
// @schemes http https

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	command := collection.NewApp("collection-server")
	command.Run()
}
