package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// padRight pads or truncates s to exactly width visible characters.
func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) > width {
		return string(runes[:width-2]) + ".."
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// colHeader renders a header cell without block padding (HeaderStyle has PaddingBottom
// which would cause each cell to wrap to its own line).
var colHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))

func shiftDate(s string, days int) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}
