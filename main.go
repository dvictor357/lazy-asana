package main

import (
	"fmt"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/config"
	"github.com/dvictor357/lazy-asana/internal/tui"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Println("lazy-asana", buildVersion())
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	app := tui.NewApp(cfg)
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// buildVersion prefers the release version, then the module version that
// `go install ...@v1.2.3` records.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
