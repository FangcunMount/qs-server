package main

import (
	"github.com/FangcunMount/qs-server/internal/apiserver"
)

func main() {
	apiserver.NewApp("qs-apiserver").Run()
}
