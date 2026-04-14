package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

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
	httpClient *http.Client
}

func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = "deepseek/deepseek-chat-v3-0324"
	}
	return &Client{
		apiKey:     apiKey,
		model:      model,
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

	req, err := http.NewRequest("POST", openRouterURL, bytes.NewReader(data))
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
		return "", fmt.Errorf("openrouter: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("openrouter: no response")
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

func (c *Client) GenerateWeekPlan(context string) (string, error) {
	msgs := []Message{
		{Role: "system", Content: `You are a project planning assistant. Generate a structured day/week plan based on the given context.

Output a JSON array of tasks. Each task should have:
- "title": string (task name)
- "description": string (2-3 sentence description)
- "hours": number (estimated actual hours, be realistic)
- "due_date": string (YYYY-MM-DD format)
- "subtasks": array of objects with "title", "description", "hours" (optional, can be empty)

Output ONLY valid JSON, no markdown fences or explanation.`},
		{Role: "user", Content: context},
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
