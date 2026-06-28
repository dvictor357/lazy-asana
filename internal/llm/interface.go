package llm

// LLMClient is the interface for the LLM client used by TUI models.
// The concrete *Client type satisfies this interface.
type LLMClient interface {
	Chat(messages []Message) (string, error)
	GenerateTaskDescription(taskTitle string) (string, error)
	GenerateWeekPlan(userContext string, meta WeekMeta) (string, error)
}
