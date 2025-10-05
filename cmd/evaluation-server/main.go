package main

import (
	"math/rand"
	"time"

	evaluation "github.com/fangcun-mount/qs-server/internal/evaluation-server"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	command := evaluation.NewApp("evaluation-server")
	command.Run()
}
