package main

import (
	"math/rand"
	"time"

	evaluation "github.com/yshujie/questionnaire-scale/internal/evaluation-server"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	command := evaluation.NewApp("evaluation-server")
	command.Run()
}
