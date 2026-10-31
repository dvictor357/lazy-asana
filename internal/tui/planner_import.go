package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/config"
	"github.com/dvictor357/lazy-asana/internal/gitimport"
)

// importState is the git import step of Task Composer: pick a date range,
// fetch commits from the configured git sources, tick the (day, repo)
// entries to add as draft tasks.
type importState struct {
	start, end textinput.Model
	field      int // 0 start, 1 end
	loading    bool
	loaded     bool // entries below are a fetch result
	seq        int  // drops results of abandoned fetches
	entries    []gitimport.Entry
	picked     []bool
	cursor     int
	warnings   []string

	// SHAs already added as tasks this session, so the same day is not
	// imported twice.
	added map[string]bool
}

type importResult struct {
	seq      int
	entries  []gitimport.Entry
	warnings []string
}

// collectCommits reads the git sources; tests swap it.
var collectCommits = func(ctx context.Context, sources []config.GitSource, r gitimport.Range) gitimport.Result {
	return gitimport.Collect(ctx, sources, r)
}

func newImportState() importState {
	mk := func() textinput.Model {
		ti := textinput.New()
		ti.Placeholder = "2026-10-26"
		ti.CharLimit = 10
		ti.Width = 12
		ti.Prompt = "  "
		return ti
	}
	return importState{start: mk(), end: mk(), added: map[string]bool{}}
}

// weekSoFar is Monday of today's week through today.
func weekSoFar(now time.Time) (string, string) {
	offset := (int(now.Weekday()) + 6) % 7 // days since Monday
	return now.AddDate(0, 0, -offset).Format("2006-01-02"), now.Format("2006-01-02")
}

// openImport shows the date range form (keeping the last range used).
func (m *PlannerModel) openImport() tea.Cmd {
	if len(m.cfg.GitSources) == 0 {
		return errorCmd("no git sources yet — add one in Settings → Git sources")
	}
	im := &m.imp
	if im.start.Value() == "" {
		s, e := weekSoFar(time.Now())
		im.start.SetValue(s)
		im.end.SetValue(e)
	}
	im.field = 0
	im.start.Focus()
	im.end.Blur()
	im.loaded = false
	im.loading = false
	m.step = plannerStepImport
	return clearError
}

// leaveImport returns to the composer and abandons any running fetch.
func (m *PlannerModel) leaveImport() {
	m.imp.loading = false
	m.imp.seq++
	m.step = plannerStepComposer
}

func (m *PlannerModel) updateImport(msg tea.Msg) (tea.Model, tea.Cmd) {
	im := &m.imp
	if r, ok := msg.(importResult); ok {
		if r.seq != im.seq || !im.loading {
			return *m, nil
		}
		im.loading = false
		im.loaded = true
		im.entries = r.entries
		im.warnings = r.warnings
		im.cursor = 0
		im.picked = make([]bool, len(r.entries))
		for i, e := range r.entries {
			im.picked[i] = !m.alreadyAdded(e)
		}
		return *m, nil
	}

	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return *m, nil
	}
	if im.loading {
		return *m, nil
	}
	if im.loaded {
		return m.updateImportList(k)
	}

	switch k.String() {
	case "tab", "shift+tab", "up", "down":
		im.field = 1 - im.field
		if im.field == 0 {
			im.start.Focus()
			im.end.Blur()
		} else {
			im.end.Focus()
			im.start.Blur()
		}
		return *m, nil
	case "enter":
		r, err := gitimport.NewRange(strings.TrimSpace(im.start.Value()), strings.TrimSpace(im.end.Value()))
		if err != nil {
			return *m, errorCmd(err.Error())
		}
		im.loading = true
		im.seq++
		seq, sources := im.seq, append([]config.GitSource(nil), m.cfg.GitSources...)
		fetch := func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			res := collectCommits(ctx, sources, r)
			return importResult{seq: seq, entries: gitimport.Group(res.Commits), warnings: res.Warnings}
		}
		return *m, tea.Batch(fetch, m.spinner.Tick, clearError)
	}

	var cmd tea.Cmd
	if im.field == 0 {
		im.start, cmd = im.start.Update(k)
	} else {
		im.end, cmd = im.end.Update(k)
	}
	return *m, cmd
}

func (m *PlannerModel) updateImportList(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	im := &m.imp
	switch k.String() {
	case "up", "k":
		if im.cursor > 0 {
			im.cursor--
		}
	case "down", "j":
		if im.cursor < len(im.entries)-1 {
			im.cursor++
		}
	case " ", "x":
		if len(im.picked) > 0 {
			im.picked[im.cursor] = !im.picked[im.cursor]
		}
	case "a":
		all := true
		for _, p := range im.picked {
			all = all && p
		}
		for i := range im.picked {
			im.picked[i] = !all
		}
	case "r":
		im.loaded = false // back to the date range
	case "enter":
		n := m.addImported()
		if n == 0 {
			return *m, errorCmd("nothing picked — space ticks an entry")
		}
		m.leaveImport()
		return *m, clearError
	}
	return *m, nil
}

// alreadyAdded reports whether every commit of e is in a task already.
func (m *PlannerModel) alreadyAdded(e gitimport.Entry) bool {
	for _, sha := range e.SHAs() {
		if !m.imp.added[sha] {
			return false
		}
	}
	return true
}

// addImported appends the picked entries as draft tasks and selects the
// first one. It returns how many were added.
func (m *PlannerModel) addImported() int {
	im := &m.imp
	first := len(m.tasks)
	for i, e := range im.entries {
		if !im.picked[i] {
			continue
		}
		m.tasks = append(m.tasks, PlanTask{
			Title:       e.Title(),
			Description: e.Description(),
			Hours:       e.Hours,
			DueDate:     e.Date,
		})
		for _, sha := range e.SHAs() {
			im.added[sha] = true
		}
	}
	n := len(m.tasks) - first
	if n > 0 {
		m.reviewCursor = first
	}
	return n
}

func (m PlannerModel) viewImport() string {
	im := m.imp
	var b strings.Builder
	b.WriteString(HeaderStyle.Render("Import from git"))
	b.WriteString("\n")

	var names []string
	for _, s := range m.cfg.GitSources {
		names = append(names, s.Label())
	}
	b.WriteString(DimStyle.Render("Sources: "+strings.Join(names, ", ")) + "\n\n")

	if !im.loaded {
		label := func(s string, i int) string {
			if im.field == i {
				return HighlightStyle.Render("> " + s)
			}
			return LabelStyle.Render("  " + s)
		}
		b.WriteString(label("From (YYYY-MM-DD)", 0) + "\n" + im.start.View() + "\n\n")
		b.WriteString(label("To (YYYY-MM-DD)", 1) + "\n" + im.end.View() + "\n\n")
		if im.loading {
			b.WriteString(m.spinner.View() + " Reading commits...\n")
			return b.String()
		}
		b.WriteString(DimStyle.Render("tab switch field • enter fetch commits • esc back"))
		return b.String()
	}

	for _, w := range im.warnings {
		b.WriteString(ErrorStyle.UnsetMarginTop().Render("! "+w) + "\n")
	}
	if len(im.warnings) > 0 {
		b.WriteString("\n")
	}
	if len(im.entries) == 0 {
		b.WriteString(DimStyle.Render(fmt.Sprintf("No commits of yours between %s and %s.", im.start.Value(), im.end.Value())))
		b.WriteString("\n\n" + DimStyle.Render("r change dates • esc back"))
		return b.String()
	}

	total, picked := 0.0, 0
	day := ""
	for i, e := range im.entries {
		if e.Date != day {
			day = e.Date
			b.WriteString(LabelStyle.Render(shortDate(day)) + "\n")
		}
		box := "[ ]"
		if im.picked[i] {
			box = "[x]"
			total += e.Hours
			picked++
		}
		line := fmt.Sprintf("%s %s %4.1fh  %2d commit%s", box, padRight(e.Repo, 18), e.Hours, len(e.Commits), plural(len(e.Commits)))
		if m.alreadyAdded(e) {
			line += "  (added)"
		}
		if i == im.cursor {
			b.WriteString(HighlightStyle.Render("> "+line) + "\n")
			for _, c := range e.Commits {
				b.WriteString(DimStyle.Render("      • "+c.Subject) + "\n")
			}
		} else {
			b.WriteString(ValueStyle.Render("  "+line) + "\n")
		}
	}
	b.WriteString("\n" + ValueStyle.Render(fmt.Sprintf("%d picked · %.1fh", picked, total)))
	b.WriteString("  " + DimStyle.Render("hours are estimates from commit size, max 8h a day — adjust before submitting") + "\n\n")
	b.WriteString(DimStyle.Render("↑↓ move • space pick • a all/none • enter add as tasks • r change dates • esc back"))
	return b.String()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
