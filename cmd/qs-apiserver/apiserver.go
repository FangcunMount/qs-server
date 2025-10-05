package main

import (
	"github.com/fangcun-mount/qs-server/internal/apiserver"
)

func main() {
	apiserver.NewApp("qs-apiserver").Run()
}
