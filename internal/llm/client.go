package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"
const deepSeekURL = "https://api.deepseek.com/chat/completions"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type Client struct {
	apiKey     string
	model      string
	baseURL    string
	name       string
	httpClient *http.Client
}

func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = "deepseek/deepseek-chat-v3-0324"
	}
	return &Client{
		apiKey:     apiKey,
		model:      model,
		baseURL:    openRouterURL,
		name:       "openrouter",
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func NewDeepSeekClient(apiKey, model string) *Client {
	if model == "" {
		model = "deepseek-chat"
	}
	return &Client{
		apiKey:     apiKey,
		model:      model,
		baseURL:    deepSeekURL,
		name:       "deepseek",
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Chat(messages []Message) (string, error) {
	reqBody := ChatRequest{
		Model:    c.model,
		Messages: messages,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/dvictor357/lazy-asana")
	req.Header.Set("X-Title", "lazy-asana")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s: HTTP %d: %s", c.name, resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("%s: no response", c.name)
	}

	return chatResp.Choices[0].Message.Content, nil
}

func (c *Client) GenerateTaskDescription(taskTitle string) (string, error) {
	msgs := []Message{
		{Role: "system", Content: "You are a project planning assistant. Generate a concise, professional task description for the given task title. The description should be 2-4 sentences explaining what needs to be done. Output ONLY the description text, nothing else."},
		{Role: "user", Content: fmt.Sprintf("Generate a description for this task: %s", taskTitle)},
	}
	return c.Chat(msgs)
}

type WeekMeta struct {
	WeekStart    string
	WeekEnd      string
	WorkDays     []string
	PlannedHours map[string]float64
}

func (c *Client) GenerateWeekPlan(userContext string, meta WeekMeta) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Planning period: %s to %s (%d workday(s))\n\n", meta.WeekStart, meta.WeekEnd, len(meta.WorkDays)))
	sb.WriteString(fmt.Sprintf("Capacity per day (max 8h/day, %dh total):\n", len(meta.WorkDays)*8))
	totalPlanned := 0.0
	for _, d := range meta.WorkDays {
		planned := meta.PlannedHours[d]
		totalPlanned += planned
		remaining := 8.0 - planned
		if remaining < 0 {
			remaining = 0
		}
		sb.WriteString(fmt.Sprintf("  %s: %.1fh already planned, %.1fh remaining\n", d, planned, remaining))
	}
	maxTotal := float64(len(meta.WorkDays)) * 8.0
	weekRemaining := maxTotal - totalPlanned
	if weekRemaining < 0 {
		weekRemaining = 0
	}
	sb.WriteString(fmt.Sprintf("\nPeriod total: %.1fh already planned, %.1fh remaining\n\n", totalPlanned, weekRemaining))
	sb.WriteString("User request:\n")
	sb.WriteString(userContext)

	msgs := []Message{
		{Role: "system", Content: `You are a project planning assistant. Generate a structured day/week plan based on the given context.

Hard constraints:
- Maximum 8 hours per day — never exceed daily remaining capacity shown
- Maximum hours = 8h × number of workdays — never exceed total remaining capacity shown
- Only schedule tasks on Mon–Fri (no weekends)
- All due_date values must fall within the provided planning period
- If remaining capacity for a day is 0, do not schedule tasks on that day

Output a JSON array of tasks. Each task should have:
- "title": string (task name)
- "description": string (2-3 sentence description)
- "hours": number (estimated hours, must fit within remaining daily capacity)
- "due_date": string (YYYY-MM-DD format, must be a workday within the given week)
- "subtasks": array of objects with "title", "description", "hours" (optional, can be empty)

Output ONLY valid JSON, no markdown fences or explanation.`},
		{Role: "user", Content: sb.String()},
	}
	return c.Chat(msgs)
}

type PlannedTask struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Hours       float64 `json:"hours"`
	DueDate     string  `json:"due_date"`
	Subtasks    []struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Hours       float64 `json:"hours"`
	} `json:"subtasks"`
}

func ParsePlan(response string) ([]PlannedTask, error) {
	var tasks []PlannedTask
	if err := json.Unmarshal([]byte(response), &tasks); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w\n\nResponse was:\n%s", err, response)
	}
	return tasks, nil
}
