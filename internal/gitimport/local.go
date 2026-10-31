package gitimport

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dvictor357/lazy-asana/internal/config"
)

// DefaultDepth is how deep a local folder is searched for repos.
const DefaultDepth = 4

// pruned directories are never searched for repos.
var pruned = map[string]bool{
	"node_modules": true, "vendor": true, ".venv": true, "venv": true,
	"dist": true, "build": true, "target": true, ".next": true,
	".cache": true, "__pycache__": true, "Pods": true,
}

// fetchLocal reads the user's commits on every branch of every repo under
// src.Path, matched on the global and the repo's own user.name/user.email.
func fetchLocal(ctx context.Context, src config.GitSource, r Range) ([]Commit, error) {
	root := config.ExpandHome(src.Path)
	depth := src.Depth
	if depth == 0 {
		depth = DefaultDepth
	}
	repos, err := findRepos(root, depth)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("no git repos found under %s (depth %d)", src.Path, depth)
	}
	global := identity(ctx, "", "--global")

	var all []Commit
	var skipped []string
	for _, repo := range repos {
		ids := append(identity(ctx, repo, "--local"), global...)
		if len(ids) == 0 {
			skipped = append(skipped, filepath.Base(repo))
			continue
		}
		commits, err := repoLog(ctx, repo, ids, r)
		if err != nil {
			return all, fmt.Errorf("%s: %w", filepath.Base(repo), err)
		}
		all = append(all, commits...)
	}
	if len(skipped) > 0 && len(all) == 0 {
		return nil, fmt.Errorf("no git user.name/user.email set (global or in %s)", strings.Join(skipped, ", "))
	}
	return all, nil
}

// findRepos lists directories holding a .git dir or file (worktrees and
// submodules use a file), at most depth levels below root.
func findRepos(root string, depth int) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("folder %s does not exist", root)
	}
	var repos []string
	base := strings.Count(filepath.Clean(root), string(filepath.Separator))
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil // unreadable subfolder: skip it
		}
		if d.Name() == ".git" {
			repos = append(repos, filepath.Dir(p))
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p != root && pruned[d.Name()] {
			return filepath.SkipDir
		}
		if strings.Count(filepath.Clean(p), string(filepath.Separator))-base > depth {
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(repos)
	return repos, err
}

// identity returns the non-empty user.name and user.email at one config
// scope. repo is empty for --global.
func identity(ctx context.Context, repo, scope string) []string {
	var ids []string
	for _, key := range []string{"user.name", "user.email"} {
		args := []string{"config", scope, key}
		if repo != "" {
			args = append([]string{"-C", repo}, args...)
		}
		out, err := exec.CommandContext(ctx, "git", args...).Output()
		if err != nil {
			continue
		}
		if v := strings.TrimSpace(string(out)); v != "" {
			ids = append(ids, v)
		}
	}
	return ids
}

const commitMark = "\x1e" // starts each commit header in our log format

// repoLog runs git log on all branches, no merges, for the given authors.
// The --since/--until window is widened by a day on each side because git
// filters on committer date; Collect trims to the author date.
func repoLog(ctx context.Context, repo string, authors []string, r Range) ([]Commit, error) {
	args := []string{"-C", repo, "log", "--all", "--no-merges", "--extended-regexp",
		"--since=" + r.Start.AddDate(0, 0, -1).Format(time.RFC3339),
		"--until=" + r.End.AddDate(0, 0, 2).Format(time.RFC3339),
		"--pretty=format:" + commitMark + "%H%x1f%aI%x1f%s",
		"--shortstat",
	}
	for _, a := range authors {
		args = append(args, "--author="+regexp.QuoteMeta(a))
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "does not have any commits") {
			return nil, nil // empty repo
		}
		return nil, fmt.Errorf("git log: %v %s", err, msg)
	}
	return parseLog(filepath.Base(repo), out), nil
}

// parseLog reads the output of repoLog.
func parseLog(repo string, out []byte) []Commit {
	var commits []Commit
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, commitMark) {
			parts := strings.SplitN(strings.TrimPrefix(line, commitMark), "\x1f", 3)
			if len(parts) != 3 {
				continue
			}
			when, err := time.Parse(time.RFC3339, parts[1])
			if err != nil {
				continue
			}
			commits = append(commits, Commit{SHA: parts[0], Repo: repo, When: when, Subject: parts[2], HasStats: true})
			continue
		}
		if len(commits) > 0 && strings.Contains(line, "changed") {
			c := &commits[len(commits)-1]
			c.Files, c.Additions, c.Deletions = parseShortstat(line)
		}
	}
	return commits
}

// parseShortstat reads " 3 files changed, 10 insertions(+), 2 deletions(-)".
func parseShortstat(line string) (files, ins, del int) {
	for _, part := range strings.Split(line, ",") {
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(fields[1], "file"):
			files = n
		case strings.HasPrefix(fields[1], "insertion"):
			ins = n
		case strings.HasPrefix(fields[1], "deletion"):
			del = n
		}
	}
	return
}
