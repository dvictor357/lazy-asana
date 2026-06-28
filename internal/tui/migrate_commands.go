package tui

import (
	"fmt"
	"net/url"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
)

// ── Commands ──────────────────────────────────────────────────────────────────

func (m MigrateModel) fetchTasks(weekNum int) tea.Cmd {
	wsGID := m.cfg.DefaultWorkspaceGID
	userGID := m.cfg.UserGID
	client := m.client
	return func() tea.Msg {
		dateStart, dateEnd := asana.WeekToDateRange(asana.CurrentYear(), weekNum)
		filters := url.Values{
			"due_on.after":  {shiftDate(dateStart, -1)},
			"due_on.before": {shiftDate(dateEnd, 1)},
		}
		if userGID != "" {
			filters["assignee.any"] = []string{userGID}
		}
		tasks, err := client.SearchTasks(wsGID, filters)
		if err != nil {
			return migrateTasksMsg{err: err}
		}
		return migrateTasksMsg{tasks: tasks, fetchedStart: dateStart, fetchedEnd: dateEnd, weekNum: weekNum}
	}
}

func (m MigrateModel) fetchProjects() tea.Cmd {
	wsGID := m.cfg.DefaultWorkspaceGID
	client := m.client
	return func() tea.Msg {
		projects, err := client.ListProjects(wsGID)
		if err != nil {
			return migrateProjsMsg{err: err}
		}
		return migrateProjsMsg{projects: projects}
	}
}

func (m MigrateModel) fetchSections(projectGID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		sections, err := client.ListSections(projectGID)
		if err != nil {
			return migrateSectionsMsg{err: err}
		}
		return migrateSectionsMsg{sections: sections}
	}
}

func (m *MigrateModel) execMoveNext(idx, moved, failed int) tea.Cmd {
	if idx >= len(m.selectedTasks) {
		return func() tea.Msg {
			return migrateExecMsg{done: true, moved: moved, failed: failed}
		}
	}
	task := m.selectedTasks[idx]
	tgtGID := m.targetProj.GID
	var sectionGID string
	if m.sectionCursor < len(m.sections) {
		sectionGID = m.sections[m.sectionCursor].GID
	}
	var srcGID string
	if m.sourceCursor < len(m.sourceOpts) {
		srcGID = m.sourceOpts[m.sourceCursor].GID
	}
	client := m.client
	return func() tea.Msg {
		if err := client.AddTaskToProject(task.GID, tgtGID, sectionGID); err != nil {
			return migrateExecMsg{nextIdx: idx + 1, moved: moved, failed: failed + 1,
				errLine: fmt.Sprintf("%s: add failed: %s", task.Name, err)}
		}
		// Only remove from source if this task is actually in that source project
		if srcGID != "" {
			inSrc := false
			for _, mem := range task.Memberships {
				if mem.Project.GID == srcGID {
					inSrc = true
					break
				}
			}
			if inSrc {
				if err := client.RemoveTaskFromProject(task.GID, srcGID); err != nil {
					return migrateExecMsg{nextIdx: idx + 1, moved: moved, failed: failed + 1,
						errLine: fmt.Sprintf("%s: remove from source failed: %s", task.Name, err)}
				}
			}
		}
		return migrateExecMsg{nextIdx: idx + 1, moved: moved + 1, failed: failed}
	}
}

func (m MigrateModel) filteredProjects() []asana.Project {
	query := strings.ToLower(strings.TrimSpace(m.projSearch.Value()))
	if query == "" {
		return m.projects
	}
	var out []asana.Project
	for _, p := range m.projects {
		if strings.Contains(strings.ToLower(p.Name), query) {
			out = append(out, p)
		}
	}
	return out
}
