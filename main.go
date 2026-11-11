// Command edelweiss-ext-system is the "System overview" extension of edelweiss.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/Pukka922/edelweiss-ext-system/internal/api"
	"github.com/Pukka922/edelweiss/sdk"
)

//go:embed manifest.json
var manifest []byte

// The version of a release: only the release build sets it, with -ldflags "-X main.version=<version>" (make release),
// and it is empty in every other build, which then reports the placeholder version of manifest.json.
var version string

// manifestWithVersion returns the manifest with its version field set to v. An empty v returns raw itself, so a build
// without a release version reports the placeholder of manifest.json.
func manifestWithVersion(raw []byte, v string) ([]byte, error) {
	if v == "" {
		return raw, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	if fields == nil { // the JSON value null unmarshals without an error
		return nil, fmt.Errorf("manifest: not a JSON object")
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("manifest: encode version %q: %w", v, err)
	}
	fields["version"] = encoded
	out, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	return out, nil
}

//go:embed all:ui/dist
var uiFiles embed.FS

func main() {
	m, err := manifestWithVersion(manifest, version)
	if err != nil {
		panic(err)
	}
	ui, err := fs.Sub(uiFiles, "ui/dist")
	if err != nil {
		panic(err)
	}
	ext := sdk.New(sdk.Options{Manifest: m, UI: ui})
	api.Register(ext.App, api.NewService())
	ext.Run()
}
