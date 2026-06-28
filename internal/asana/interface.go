package asana

import "net/url"

// AsanaClient is the interface for the Asana API client used by TUI models.
// The concrete *Client type satisfies this interface.
type AsanaClient interface {
	SearchTasks(workspaceGID string, filters url.Values) ([]Task, error)
	CreateTask(input map[string]interface{}) (*Task, error)
	CreateSubtask(parentGID string, input map[string]interface{}) (*Task, error)
	GetMe() (*User, error)
	ListWorkspaces() ([]Workspace, error)
	ListProjects(workspaceGID string) ([]Project, error)
	GetCustomFieldSettings(projectGID string) ([]CustomFieldSetting, error)
	ListSections(projectGID string) ([]Section, error)
	AddTaskToProject(taskGID, projectGID, sectionGID string) error
	RemoveTaskFromProject(taskGID, projectGID string) error
	GetWeekTasks(workspaceGID, projectGID, userGID, from, to string) ([]Task, error)
}
