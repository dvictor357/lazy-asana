package asana

import (
	"encoding/json"
	"time"
)

type Task struct {
	GID          string        `json:"gid"`
	Name         string        `json:"name"`
	Notes        string        `json:"notes"`
	Assignee     *UserRef      `json:"assignee,omitempty"`
	Projects     []ProjectRef  `json:"projects,omitempty"`
	DueOn        string        `json:"due_on,omitempty"`
	Completed    bool          `json:"completed"`
	CompletedAt  *time.Time    `json:"completed_at,omitempty"`
	CustomFields []CustomField `json:"custom_fields,omitempty"`
	Parent       *TaskRef      `json:"parent,omitempty"`
	Subtasks     []Task        `json:"subtasks,omitempty"`
}

type TaskRef struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

type UserRef struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

type ProjectRef struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

type Workspace struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

type Project struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

type CustomField struct {
	GID         string       `json:"gid"`
	Name        string       `json:"name"`
	Type        string       `json:"type"`
	NumberValue *float64     `json:"number_value,omitempty"`
	TextValue   *string      `json:"text_value,omitempty"`
	EnumValue   *EnumOption  `json:"enum_value,omitempty"`
	EnumOptions []EnumOption `json:"enum_options,omitempty"`
}

type EnumOption struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

type CustomFieldSetting struct {
	CustomField CustomField `json:"custom_field"`
	IsImportant bool        `json:"is_important"`
}

type User struct {
	GID   string `json:"gid"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type ErrorResponse struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (e ErrorResponse) Error() string {
	if len(e.Errors) > 0 {
		return e.Errors[0].Message
	}
	return "unknown asana error"
}

type asanaResponse struct {
	Data     json.RawMessage `json:"data"`
	NextPage *struct {
		Offset string `json:"offset"`
		Path   string `json:"path"`
		URI    string `json:"uri"`
	} `json:"next_page"`
}
