package gitimport

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dvictor357/lazy-asana/internal/config"
)

// maxStatCalls bounds the per-commit requests made for diff stats; above it
// GitHub commits are imported without stats.
const maxStatCalls = 150

func githubBase(src config.GitSource) string {
	base := strings.TrimRight(src.BaseURL, "/")
	if base == "" {
		base = config.GitHubAPI
	}
	return base
}

// fetchGitHub uses commit search (author + author-date). GitHub only indexes
// default branches, so commits on unmerged feature branches are not found.
func fetchGitHub(ctx context.Context, src config.GitSource, r Range) ([]Commit, error) {
	api := githubBase(src)
	h := map[string]string{
		"Authorization":        "Bearer " + src.Token,
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}

	var me struct {
		Login string `json:"login"`
	}
	if _, err := getJSON(ctx, api+"/user", h, &me); err != nil {
		return nil, err
	}

	type item struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
		Parents    []struct{} `json:"parents"`
		Repository struct {
			Name     string `json:"name"`
			FullName string `json:"full_name"`
		} `json:"repository"`
	}

	var all []Commit
	fullName := map[string]string{} // SHA -> owner/repo, for the stats call
	q := fmt.Sprintf("author:%s author-date:%s..%s", me.Login,
		r.Start.AddDate(0, 0, -1).Format("2006-01-02"), r.End.AddDate(0, 0, 1).Format("2006-01-02"))
	// Search returns at most 1000 results (10 pages of 100).
	for page := 1; page <= 10; page++ {
		v := url.Values{"q": {q}, "sort": {"author-date"}, "order": {"asc"},
			"per_page": {"100"}, "page": {fmt.Sprint(page)}}
		var res struct {
			Items []item `json:"items"`
		}
		if _, err := getJSON(ctx, api+"/search/commits?"+v.Encode(), h, &res); err != nil {
			return all, err
		}
		for _, it := range res.Items {
			if len(it.Parents) > 1 {
				continue // merge commit
			}
			all = append(all, Commit{SHA: it.SHA, Repo: it.Repository.Name,
				When: it.Commit.Author.Date, Subject: firstLine(it.Commit.Message)})
			fullName[it.SHA] = it.Repository.FullName
		}
		if len(res.Items) < 100 {
			break
		}
	}

	// Diff stats need one call per commit; skip them for big ranges.
	if len(all) <= maxStatCalls {
		for i := range all {
			if !r.Contains(all[i].When) {
				continue
			}
			var detail struct {
				Stats struct {
					Additions int `json:"additions"`
					Deletions int `json:"deletions"`
				} `json:"stats"`
				Files []struct{} `json:"files"`
			}
			u := fmt.Sprintf("%s/repos/%s/commits/%s", api, fullName[all[i].SHA], all[i].SHA)
			if _, err := getJSON(ctx, u, h, &detail); err != nil {
				continue // stats are optional
			}
			all[i].Additions, all[i].Deletions = detail.Stats.Additions, detail.Stats.Deletions
			all[i].Files, all[i].HasStats = len(detail.Files), true
		}
	}
	return all, nil
}
