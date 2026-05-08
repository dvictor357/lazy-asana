package asana

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://app.asana.com/api/1.0"

type Client struct {
	token      string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) doRequest(method, path string, body interface{}) (json.RawMessage, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if json.Unmarshal(data, &errResp) == nil && len(errResp.Errors) > 0 {
			return nil, fmt.Errorf("asana: %s", errResp.Error())
		}
		return nil, fmt.Errorf("asana: HTTP %d: %s", resp.StatusCode, string(data))
	}

	var wrapper asanaResponse
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Data, nil
}

func (c *Client) doPaginatedRequest(path string, params url.Values) (json.RawMessage, error) {
	var allResults []json.RawMessage

	for {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		fullPath := path
		if len(params) > 0 {
			fullPath += sep + params.Encode()
		}

		req, err := http.NewRequest("GET", baseURL+fullPath, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if resp.StatusCode >= 400 {
			var errResp ErrorResponse
			if json.Unmarshal(data, &errResp) == nil && len(errResp.Errors) > 0 {
				return nil, fmt.Errorf("asana: %s", errResp.Error())
			}
			return nil, fmt.Errorf("asana: HTTP %d: %s", resp.StatusCode, string(data))
		}

		var wrapper asanaResponse
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, err
		}

		var items []json.RawMessage
		if err := json.Unmarshal(wrapper.Data, &items); err != nil {
			return wrapper.Data, nil
		}
		allResults = append(allResults, items...)

		if wrapper.NextPage == nil || wrapper.NextPage.Offset == "" {
			break
		}
		params.Set("offset", wrapper.NextPage.Offset)
	}

	result, err := json.Marshal(allResults)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) GetMe() (*User, error) {
	data, err := c.doRequest("GET", "/users/me", nil)
	if err != nil {
		return nil, err
	}
	var user User
	return &user, json.Unmarshal(data, &user)
}

func (c *Client) ListWorkspaces() ([]Workspace, error) {
	params := url.Values{
		"limit":      {"100"},
		"opt_fields": {"name"},
	}
	data, err := c.doPaginatedRequest("/workspaces", params)
	if err != nil {
		return nil, err
	}
	var ws []Workspace
	return ws, json.Unmarshal(data, &ws)
}

func (c *Client) ListProjects(workspaceGID string) ([]Project, error) {
	params := url.Values{
		"workspace":  {workspaceGID},
		"limit":      {"100"},
		"opt_fields": {"name"},
		"archived":   {"false"},
	}
	data, err := c.doPaginatedRequest("/projects", params)
	if err != nil {
		return nil, err
	}
	var projects []Project
	return projects, json.Unmarshal(data, &projects)
}

func (c *Client) GetCustomFieldSettings(projectGID string) ([]CustomFieldSetting, error) {
	params := url.Values{
		"limit":      {"100"},
		"opt_fields": {"custom_field.name,custom_field.type,custom_field.gid,custom_field.enum_options,is_important"},
	}
	path := fmt.Sprintf("/projects/%s/custom_field_settings", projectGID)
	data, err := c.doPaginatedRequest(path, params)
	if err != nil {
		return nil, err
	}
	var settings []CustomFieldSetting
	return settings, json.Unmarshal(data, &settings)
}

func (c *Client) SearchTasks(workspaceGID string, filters url.Values) ([]Task, error) {
	params := url.Values{
		"limit":      {"100"},
		"opt_fields": {"name,notes,due_on,completed,assignee.name,custom_fields.name,custom_fields.number_value,custom_fields.gid,memberships.project.gid,memberships.project.name,memberships.section.gid,memberships.section.name"},
	}
	for k, v := range filters {
		params[k] = v
	}
	path := fmt.Sprintf("/workspaces/%s/tasks/search", workspaceGID)
	data, err := c.doRequest("GET", path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	return tasks, json.Unmarshal(data, &tasks)
}

func (c *Client) GetWeekTasks(workspaceGID, projectGID, userGID, from, to string) ([]Task, error) {
	fromDate, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil, err
	}
	toDate, err := time.Parse("2006-01-02", to)
	if err != nil {
		return nil, err
	}
	filters := url.Values{
		"due_on.after":  {fromDate.AddDate(0, 0, -1).Format("2006-01-02")},
		"due_on.before": {toDate.AddDate(0, 0, 1).Format("2006-01-02")},
		"projects.any":  {projectGID},
	}
	if userGID != "" {
		filters.Set("assignee.any", userGID)
	}
	return c.SearchTasks(workspaceGID, filters)
}

func (c *Client) ListTasksByProject(projectGID string) ([]Task, error) {
	params := url.Values{
		"limit":           {"100"},
		"opt_fields":      {"name,notes,due_on,completed,assignee.name,custom_fields.name,custom_fields.number_value,custom_fields.gid"},
		"completed_since": {"now"},
	}
	path := fmt.Sprintf("/projects/%s/tasks", projectGID)
	data, err := c.doPaginatedRequest(path, params)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	return tasks, json.Unmarshal(data, &tasks)
}

func (c *Client) CreateTask(input map[string]interface{}) (*Task, error) {
	body := map[string]interface{}{
		"data": input,
	}
	data, err := c.doRequest("POST", "/tasks", body)
	if err != nil {
		return nil, err
	}
	var task Task
	return &task, json.Unmarshal(data, &task)
}

func (c *Client) CreateSubtask(parentGID string, input map[string]interface{}) (*Task, error) {
	body := map[string]interface{}{
		"data": input,
	}
	path := fmt.Sprintf("/tasks/%s/subtasks", parentGID)
	data, err := c.doRequest("POST", path, body)
	if err != nil {
		return nil, err
	}
	var task Task
	return &task, json.Unmarshal(data, &task)
}

func (c *Client) GetTask(taskGID string) (*Task, error) {
	params := url.Values{
		"opt_fields": {"name,notes,due_on,completed,assignee.name,custom_fields.name,custom_fields.number_value,custom_fields.gid,subtasks"},
	}
	path := fmt.Sprintf("/tasks/%s?%s", taskGID, params.Encode())
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var task Task
	return &task, json.Unmarshal(data, &task)
}

func (c *Client) GetSubtasks(taskGID string) ([]Task, error) {
	params := url.Values{
		"limit":      {"100"},
		"opt_fields": {"name,notes,due_on,completed,custom_fields.name,custom_fields.number_value,custom_fields.gid"},
	}
	path := fmt.Sprintf("/tasks/%s/subtasks", taskGID)
	data, err := c.doPaginatedRequest(path, params)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	return tasks, json.Unmarshal(data, &tasks)
}

func (c *Client) ListSections(projectGID string) ([]Section, error) {
	params := url.Values{
		"limit":      {"100"},
		"opt_fields": {"name"},
	}
	path := fmt.Sprintf("/projects/%s/sections", projectGID)
	data, err := c.doPaginatedRequest(path, params)
	if err != nil {
		return nil, err
	}
	var sections []Section
	return sections, json.Unmarshal(data, &sections)
}

// AddTaskToProject adds a task to a project, optionally placing it in a section.
// Pass sectionGID="" to use the project's default section.
func (c *Client) AddTaskToProject(taskGID, projectGID, sectionGID string) error {
	data := map[string]interface{}{"project": projectGID}
	if sectionGID != "" {
		data["section"] = sectionGID
	}
	body := map[string]interface{}{"data": data}
	_, err := c.doRequest("POST", fmt.Sprintf("/tasks/%s/addProject", taskGID), body)
	return err
}

func (c *Client) RemoveTaskFromProject(taskGID, projectGID string) error {
	body := map[string]interface{}{
		"data": map[string]interface{}{"project": projectGID},
	}
	_, err := c.doRequest("POST", fmt.Sprintf("/tasks/%s/removeProject", taskGID), body)
	return err
}

func (c *Client) UpdateTask(taskGID string, input map[string]interface{}) (*Task, error) {
	body := map[string]interface{}{
		"data": input,
	}
	path := fmt.Sprintf("/tasks/%s", taskGID)
	data, err := c.doRequest("PUT", path, body)
	if err != nil {
		return nil, err
	}
	var task Task
	return &task, json.Unmarshal(data, &task)
}

func ParseActualHours(fields []CustomField, fieldGID string) float64 {
	for _, f := range fields {
		if f.GID == fieldGID && f.NumberValue != nil {
			return *f.NumberValue
		}
	}
	return 0
}

func WeekToDateRange(year, week int) (string, string) {
	jan1 := fmt.Sprintf("%d-01-01", year)
	t, _ := parseDate(jan1)
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	daysToMonday := 1 - weekday
	if daysToMonday > 0 {
		daysToMonday -= 7
	}
	monday := t.AddDate(0, 0, daysToMonday+(week-1)*7)
	sunday := monday.AddDate(0, 0, 6)
	return monday.Format("2006-01-02"), sunday.Format("2006-01-02")
}

func parseDate(s string) (Time, error) {
	t, err := time.Parse("2006-01-02", s)
	return t, err
}

type Time = time.Time

func CurrentYear() int {
	return time.Now().Year()
}

func CurrentWeek() int {
	_, w := time.Now().ISOWeek()
	return w
}

func ValidateDate(s string) error {
	_, err := time.Parse("2006-01-02", s)
	return err
}

func ValidateWeek(year, week int) error {
	if week < 1 || week > 53 {
		return fmt.Errorf("week must be between 1 and 53")
	}
	return nil
}

func FormatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
