package gitimport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dvictor357/lazy-asana/internal/config"
)

// maxPages bounds every paginated API walk (100 items a page).
const maxPages = 20

// getJSON GETs u with the given headers and decodes the body into v. It
// returns the response headers for pagination.
func getJSON(ctx context.Context, u string, headers map[string]string, v any) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, val := range headers {
		req.Header.Set(k, val)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, apiError(resp.StatusCode, req.URL.Path, body)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return nil, fmt.Errorf("%s: unexpected response: %v", req.URL.Path, err)
	}
	return resp.Header, nil
}

// apiError names the status, the endpoint and a hint; it never echoes headers
// (the token lives there).
func apiError(status int, path string, body []byte) error {
	var payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	_ = json.Unmarshal(body, &payload)
	msg := payload.Message
	if msg == "" {
		msg = payload.Error
	}
	hint := ""
	switch status {
	case http.StatusUnauthorized:
		hint = " — token invalid or expired"
	case http.StatusForbidden:
		hint = " — token lacks a scope (GitLab: read_api; GitHub: repo) or rate limited"
	case http.StatusNotFound:
		hint = " — check the base URL"
	}
	if msg != "" {
		return fmt.Errorf("%s: HTTP %d %s%s", path, status, msg, hint)
	}
	return fmt.Errorf("%s: HTTP %d%s", path, status, hint)
}

func gitlabBase(src config.GitSource) string {
	base := strings.TrimRight(src.BaseURL, "/")
	if base == "" {
		base = config.GitLabDotCom
	}
	return base + "/api/v4"
}

// fetchGitLab finds the projects you pushed to in the range (from your
// events), then reads each project's commits on all branches and keeps the
// ones whose author email or name is yours.
func fetchGitLab(ctx context.Context, src config.GitSource, r Range) ([]Commit, error) {
	api := gitlabBase(src)
	h := map[string]string{"PRIVATE-TOKEN": src.Token}

	var me struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Email       string `json:"email"`
		PublicEmail string `json:"public_email"`
		CommitEmail string `json:"commit_email"`
	}
	if _, err := getJSON(ctx, api+"/user", h, &me); err != nil {
		return nil, err
	}
	mine := map[string]bool{}
	for _, e := range []string{me.Email, me.PublicEmail, me.CommitEmail} {
		if e != "" {
			mine[strings.ToLower(e)] = true
		}
	}
	var extra []struct {
		Email string `json:"email"`
	}
	if _, err := getJSON(ctx, api+"/user/emails", h, &extra); err == nil {
		for _, e := range extra {
			mine[strings.ToLower(e.Email)] = true
		}
	}

	// Projects with your pushes in the range (after/before are exclusive).
	projectIDs := []int{}
	seenProject := map[int]bool{}
	for page := 1; page <= maxPages; page++ {
		q := url.Values{
			"action":   {"pushed"},
			"after":    {r.Start.AddDate(0, 0, -2).Format("2006-01-02")},
			"before":   {r.End.AddDate(0, 0, 2).Format("2006-01-02")},
			"per_page": {"100"},
			"page":     {fmt.Sprint(page)},
		}
		var events []struct {
			ProjectID int `json:"project_id"`
		}
		hdr, err := getJSON(ctx, fmt.Sprintf("%s/users/%d/events?%s", api, me.ID, q.Encode()), h, &events)
		if err != nil {
			return nil, err
		}
		for _, e := range events {
			if e.ProjectID != 0 && !seenProject[e.ProjectID] {
				seenProject[e.ProjectID] = true
				projectIDs = append(projectIDs, e.ProjectID)
			}
		}
		if hdr.Get("X-Next-Page") == "" && len(events) < 100 {
			break
		}
	}

	var all []Commit
	for _, id := range projectIDs {
		var project struct {
			Path string `json:"path"`
		}
		if _, err := getJSON(ctx, fmt.Sprintf("%s/projects/%d", api, id), h, &project); err != nil {
			return all, err
		}
		for page := 1; page <= maxPages; page++ {
			q := url.Values{
				"all":        {"true"},
				"with_stats": {"true"},
				"since":      {r.Start.AddDate(0, 0, -1).Format(time.RFC3339)},
				"until":      {r.End.AddDate(0, 0, 2).Format(time.RFC3339)},
				"per_page":   {"100"},
				"page":       {fmt.Sprint(page)},
			}
			var commits []struct {
				ID          string    `json:"id"`
				Title       string    `json:"title"`
				Message     string    `json:"message"`
				AuthorName  string    `json:"author_name"`
				AuthorEmail string    `json:"author_email"`
				AuthoredAt  time.Time `json:"authored_date"`
				ParentIDs   []string  `json:"parent_ids"`
				Stats       *struct {
					Additions int `json:"additions"`
					Deletions int `json:"deletions"`
				} `json:"stats"`
			}
			hdr, err := getJSON(ctx, fmt.Sprintf("%s/projects/%d/repository/commits?%s", api, id, q.Encode()), h, &commits)
			if err != nil {
				return all, fmt.Errorf("%s: %w", project.Path, err)
			}
			for _, c := range commits {
				if len(c.ParentIDs) > 1 {
					continue // merge commit
				}
				if !mine[strings.ToLower(c.AuthorEmail)] && (me.Name == "" || c.AuthorName != me.Name) {
					continue
				}
				subject := c.Title
				if subject == "" {
					subject = firstLine(c.Message)
				}
				cm := Commit{SHA: c.ID, Repo: project.Path, When: c.AuthoredAt, Subject: subject}
				if c.Stats != nil {
					cm.Additions, cm.Deletions, cm.HasStats = c.Stats.Additions, c.Stats.Deletions, true
				}
				all = append(all, cm)
			}
			if hdr.Get("X-Next-Page") == "" && len(commits) < 100 {
				break
			}
		}
	}
	return all, nil
}
