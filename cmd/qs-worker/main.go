package main

import (
	"github.com/FangcunMount/qs-server/internal/worker"
)

// @title QS Worker
// @version 1.0
// @description Questionnaire worker
// @BasePath /api/v1
// @schemes http https

func main() {
	worker.NewApp("qs-worker").Run()
}
