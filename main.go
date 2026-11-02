package main

import (
	"embed"
	"os"

	"audio-ui/internal/agenthost"
	"audio-ui/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is injected from VERSION through the Wails Taskfile.
var version = "dev"

func main() {
	if agenthost.IsHost() {
		os.Exit(agenthost.Run(os.Args[1:]))
	}
	os.Exit(app.Run(app.Config{
		Assets:  assets,
		Version: version,
	}))
}
