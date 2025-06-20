package main

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver"
)

func main() {
	apiserver.NewApp("qs-apiserver").Run()
}
