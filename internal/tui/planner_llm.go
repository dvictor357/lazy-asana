package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/llm"
)

type llmPlanResult struct {
	tasks []llm.PlannedTask
	err   string
}

func (m *PlannerModel) updateLLMPlan(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.llmGenerating {
		switch t := msg.(type) {
		case llmPlanResult:
			m.llmGenerating = false
			if t.err != "" {
				m.llmErr = t.err
				return *m, nil
			}
			for _, pt := range t.tasks {
				m.tasks = append(m.tasks, PlanTask{
					Title:       pt.Title,
					Description: pt.Description,
					Hours:       pt.Hours,
					DueDate:     pt.DueDate,
					Subtasks:    convertSubtasks(pt.Subtasks),
				})
			}
			m.step = plannerStepMenu
			return *m, nil
		}
		return *m, nil
	}

	k, ok := msg.(tea.KeyMsg)
	if !ok {
		switch m.llmFieldCursor {
		case 0:
			var cmd tea.Cmd
			m.llmStartDate, cmd = m.llmStartDate.Update(msg)
			return *m, cmd
		case 2:
			var cmd tea.Cmd
			m.llmContext, cmd = m.llmContext.Update(msg)
			return *m, cmd
		}
		return *m, nil
	}

	switch k.String() {
	case "tab":
		m.cycleLLMField(1)
		return *m, nil
	case "shift+tab":
		m.cycleLLMField(-1)
		return *m, nil

	case "left":
		if m.llmFieldCursor == 1 && m.planDays > 1 {
			m.planDays--
		}
		return *m, nil

	case "right":
		if m.llmFieldCursor == 1 && m.planDays < 5 {
			m.planDays++
		}
		return *m, nil

	case "enter":
		if m.llmFieldCursor < 2 {
			m.cycleLLMField(1)
			return *m, nil
		}
		if m.llmContext.Value() == "" {
			return *m, nil
		}
		m.llmGenerating = true
		userContext := m.llmContext.Value()
		startDateStr := m.llmStartDate.Value()
		planDays := m.planDays
		asanaCli := m.asanaCli
		cfg := m.cfg
		llmCli := m.llmCli
		return *m, tea.Batch(func() tea.Msg {
			planStart, planEnd, workDays := workDaysFrom(startDateStr, planDays)

			plannedHours := map[string]float64{}
			if asanaCli != nil && cfg.DefaultWorkspaceGID != "" && cfg.DefaultProjectGID != "" {
				tasks, err := asanaCli.GetWeekTasks(
					cfg.DefaultWorkspaceGID,
					cfg.DefaultProjectGID,
					cfg.UserGID,
					planStart,
					planEnd,
				)
				if err == nil {
					for _, t := range tasks {
						if t.DueOn != "" {
							h := asana.ParseActualHours(t.CustomFields, cfg.ActualHoursFieldGID)
							plannedHours[t.DueOn] += h
						}
					}
				}
			}

			meta := llm.WeekMeta{
				WeekStart:    planStart,
				WeekEnd:      planEnd,
				WorkDays:     workDays,
				PlannedHours: plannedHours,
			}

			resp, err := llmCli.GenerateWeekPlan(userContext, meta)
			if err != nil {
				return llmPlanResult{err: err.Error()}
			}
			tasks, err := llm.ParsePlan(resp)
			if err != nil {
				return llmPlanResult{err: err.Error()}
			}

			tasks = enforceHourCaps(tasks, plannedHours, planDays)

			return llmPlanResult{tasks: tasks}
		}, m.spinner.Tick)

	case "ctrl+s":
		return *m, m.handleLLMSubmit()
	}

	switch m.llmFieldCursor {
	case 0:
		var cmd tea.Cmd
		m.llmStartDate, cmd = m.llmStartDate.Update(msg)
		return *m, cmd
	case 2:
		var cmd tea.Cmd
		m.llmContext, cmd = m.llmContext.Update(msg)
		return *m, cmd
	}
	return *m, nil
}

func (m *PlannerModel) cycleLLMField(dir int) {
	const n = 3
	m.llmStartDate.Blur()
	m.llmContext.Blur()
	m.llmFieldCursor = ((m.llmFieldCursor + dir) % n + n) % n
	switch m.llmFieldCursor {
	case 0:
		m.llmStartDate.Focus()
	case 2:
		m.llmContext.Focus()
	}
}

// workDaysFrom returns n workdays (Mon–Fri) starting from startDateStr (YYYY-MM-DD).
// Falls back to today if startDateStr is empty or invalid.
// If start falls on a weekend, advances to the following Monday.
func workDaysFrom(startDateStr string, n int) (start, end string, days []string) {
	var cur time.Time
	if t, err := time.Parse("2006-01-02", startDateStr); err == nil {
		cur = t
	} else {
		cur = time.Now()
	}

	// advance past weekend
	switch int(cur.Weekday()) {
	case 0: // Sunday
		cur = cur.AddDate(0, 0, 1)
	case 6: // Saturday
		cur = cur.AddDate(0, 0, 2)
	}

	for len(days) < n {
		wd := int(cur.Weekday())
		if wd != 0 && wd != 6 {
			days = append(days, cur.Format("2006-01-02"))
		}
		cur = cur.AddDate(0, 0, 1)
	}

	return days[0], days[len(days)-1], days
}

// enforceHourCaps caps each task's hours so no day exceeds 8h and total does not exceed n*8h.
func enforceHourCaps(tasks []llm.PlannedTask, alreadyPlanned map[string]float64, planDays int) []llm.PlannedTask {
	maxTotal := float64(planDays) * 8.0

	dayHours := make(map[string]float64)
	total := 0.0
	for d, h := range alreadyPlanned {
		dayHours[d] = h
		total += h
	}

	for i := range tasks {
		d := tasks[i].DueDate
		if d == "" || tasks[i].Hours <= 0 {
			continue
		}
		dayRemaining := 8.0 - dayHours[d]
		if dayRemaining < 0 {
			dayRemaining = 0
		}
		totalRemaining := maxTotal - total
		if totalRemaining < 0 {
			totalRemaining = 0
		}
		cap := dayRemaining
		if totalRemaining < cap {
			cap = totalRemaining
		}
		if tasks[i].Hours > cap {
			tasks[i].Hours = cap
		}
		dayHours[d] += tasks[i].Hours
		total += tasks[i].Hours
	}
	return tasks
}

func convertSubtasks(subs []struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Hours       float64 `json:"hours"`
}) []PlanTask {
	result := make([]PlanTask, len(subs))
	for i, s := range subs {
		result[i] = PlanTask{Title: s.Title, Description: s.Description, Hours: s.Hours}
	}
	return result
}
