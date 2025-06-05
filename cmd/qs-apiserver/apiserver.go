// apiserver is the api server for iam-apiserver service.
// it is responsible for serving the platform RESTful resource management.
package main

import (
	"math/rand"
	"time"

	_ "go.uber.org/automaxprocs"

	_ "go.uber.org/automaxprocs"

	"github.com/yshujie/questionnaire-scale/internal/apiserver"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	apiserver.NewApp("qs-apiserver").Run()
}
