package main

import (
	"math/rand"
	"os"
	"time"

	evaluation "github.com/yshujie/questionnaire-scale/internal/evaluation-server"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	if len(os.Args) != 1 {
		os.Args = []string{os.Args[0]}
	}

	command := evaluation.NewApp("evaluation-server")
	command.Run()
}
