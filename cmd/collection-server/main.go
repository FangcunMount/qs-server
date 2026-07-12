package main

import (
	collection "github.com/FangcunMount/qs-server/internal/collection-server"
	_ "github.com/FangcunMount/qs-server/internal/collection-server/docs"
)

// @title Collection Server
// @version 1.0
// @description Questionnaire collection/BFF layer
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @security BearerAuth

func main() {
	collection.NewApp("collection-server").Run()
}
