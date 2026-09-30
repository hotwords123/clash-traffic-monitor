package webui

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var files embed.FS

func Files() fs.FS {
	result, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return result
}
