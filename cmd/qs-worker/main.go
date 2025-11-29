package main

import (
	"github.com/FangcunMount/qs-server/internal/worker"
)

func main() {
	worker.NewApp("qs-worker").Run()
}
