// Command edelweiss-ext-system is the "System overview" extension of edelweiss.
package main

import (
	"embed"
	"io/fs"

	"github.com/Pukka922/edelweiss-ext-system/internal/api"
	"github.com/Pukka922/edelweiss/sdk"
)

//go:embed manifest.json
var manifest []byte

//go:embed all:ui/dist
var uiFiles embed.FS

func main() {
	ui, err := fs.Sub(uiFiles, "ui/dist")
	if err != nil {
		panic(err)
	}
	ext := sdk.New(sdk.Options{Manifest: manifest, UI: ui})
	api.Register(ext.App, api.NewService())
	ext.Run()
}
