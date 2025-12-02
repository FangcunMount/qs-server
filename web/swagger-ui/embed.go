package swaggerui

import (
	"embed"
	"io/fs"
)

// Dist embeds the swagger-ui distribution files.
//
//go:embed swagger-ui-dist/*
var Dist embed.FS

// DistFS exposes the swagger-ui dist subtree for static serving.
var DistFS fs.FS

func init() {
	sub, err := fs.Sub(Dist, "swagger-ui-dist")
	if err != nil {
		panic(err)
	}
	DistFS = sub
}
