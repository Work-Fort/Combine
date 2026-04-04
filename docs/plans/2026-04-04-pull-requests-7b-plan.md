# Pull Requests Plan 7b: Diff, Merge, Mergeability, and Webhook Events

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add git operations for PR diffs, mergeability checks, and merge execution (merge/squash/rebase). Expose diff/commits/files/merge API endpoints. Parse commit keywords to auto-close issues. Fire PR webhook events.

**Architecture:** Git operations in `internal/infra/git/` wrap shell commands for merge-tree and merge execution. Backend methods in `internal/app/backend/` orchestrate merge logic. REST handlers in `internal/infra/httpapi/`. Webhook events follow the existing issue pattern.

**Tech Stack:** Go, git CLI (merge-tree, commit-tree, merge, rebase), git-module (MergeBase, Diff, Log), regexp for keyword parsing

**Prerequisites:** Plan 7a completed (PR domain model, store, CRUD API).

---

## Phase A: Git Diff Between Branches

### Task 1: Add branch diff operations to git package

**Files:**
- Create: `internal/infra/git/merge.go`

**Step 1: Add merge-base, branch diff, and mergeability helpers:**

```go
package git

import (
    "fmt"
    "os/exec"
    "strings"

    "github.com/aymanbagabas/git-module"
)

// MergeBase returns the merge-base commit hash between two refs.
func (r *Repository) MergeBase(base, head string) (string, error) {
    return r.Repository.MergeBase(base, head)
}

// DiffBranches returns the diff between two branches (from merge-base to head).
func (r *Repository) DiffBranches(base, head string) (*Diff, error) {
    mergeBase, err := r.MergeBase(base, head)
    if err != nil {
        return nil, fmt.Errorf("merge base: %w", err)
    }

    rev := mergeBase + ".." + head
    ddiff, err := r.Repository.Diff(rev, DiffMaxFiles, DiffMaxFileLines, DiffMaxLineChars, git.DiffOptions{
        CommandOptions: git.CommandOptions{
            Envs: []string{"GIT_CONFIG_GLOBAL=/dev/null"},
        },
    })
    if err != nil {
        return nil, fmt.Errorf("diff: %w", err)
    }
    return toDiff(ddiff), nil
}

// CommitsBetween returns commits between merge-base and head (reverse chronological).
func (r *Repository) CommitsBetween(base, head string) ([]*git.Commit, error) {
    mergeBase, err := r.MergeBase(base, head)
    if err != nil {
        return nil, fmt.Errorf("merge base: %w", err)
    }

    rev := mergeBase + ".." + head
    return r.Repository.Log(rev)
}

// ChangedFiles represents a file changed in a diff with stats.
type ChangedFile struct {
    Filename  string `json:"filename"`
    Additions int    `json:"additions"`
    Deletions int    `json:"deletions"`
    Status    string `json:"status"` // "added", "modified", "deleted", "renamed"
}

// ChangedFilesBetween returns the list of changed files between two branches.
func (r *Repository) ChangedFilesBetween(base, head string) ([]ChangedFile, error) {
    diff, err := r.DiffBranches(base, head)
    if err != nil {
        return nil, err
    }

    files := make([]ChangedFile, 0, len(diff.Files))
    for _, f := range diff.Files {
        cf := ChangedFile{
            Filename:  f.Name,
            Additions: f.NumAdditions(),
            Deletions: f.NumDeletions(),
        }
        from, to := f.Files()
        switch {
        case from == nil && to != nil:
            cf.Status = "added"
        case from != nil && to == nil:
            cf.Status = "deleted"
        case from != nil && to != nil && from.Name() != to.Name():
            cf.Status = "renamed"
        default:
            cf.Status = "modified"
        }
        files = append(files, cf)
    }
    return files, nil
}

// IsMergeable checks if head can be cleanly merged into base using git merge-tree.
// Requires Git >= 2.38.
func (r *Repository) IsMergeable(base, head string) (bool, error) {
    cmd := exec.Command("git", "merge-tree", "--write-tree", base, head)
    cmd.Dir = r.Path
    cmd.Env = append(cmd.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
    err := cmd.Run()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            // Non-zero exit = conflicts.
            if exitErr.ExitCode() == 1 {
                return false, nil
            }
        }
        return false, fmt.Errorf("merge-tree: %w", err)
    }
    return true, nil
}
```

**Verification:** Write a simple test that creates a repo with two branches and checks `DiffBranches`, `CommitsBetween`, and `IsMergeable`.

---

## Phase B: Merge Execution

### Task 2: Add merge execution operations

**Files:**
- Modify: `internal/infra/git/merge.go` (append to file from Task 1)

**Step 1: Add merge execution for bare repos:**

For bare repos, we cannot use `git merge` directly. Instead:

1. Use `git merge-tree --write-tree` to compute the result tree
2. Use `git commit-tree` to create a merge commit with two parents
3. Update the target ref

```go
// MergeResult contains the result of a merge operation.
type MergeResult struct {
    MergeCommit string // Hash of the merge commit
    TreeHash    string // Hash of the result tree
}

// MergeBranches performs a merge commit of head into base on a bare repo.
// Returns the merge commit hash.
func (r *Repository) MergeBranches(base, head, message string) (*MergeResult, error) {
    // Get the result tree from merge-tree.
    out, err := exec.Command("git", "merge-tree", "--write-tree", base, head).
        CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("merge-tree failed (conflicts?): %w: %s", err, out)
    }
    treeHash := strings.TrimSpace(string(out))

    // Get parent commit hashes.
    baseHash, err := r.ShowRefVerify("refs/heads/" + base)
    if err != nil {
        return nil, fmt.Errorf("resolve base: %w", err)
    }
    headHash, err := r.ShowRefVerify("refs/heads/" + head)
    if err != nil {
        return nil, fmt.Errorf("resolve head: %w", err)
    }

    // Create merge commit.
    cmd := exec.Command("git", "commit-tree", treeHash,
        "-p", baseHash, "-p", headHash, "-m", message)
    cmd.Dir = r.Path
    cmd.Env = append(cmd.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
    commitOut, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("commit-tree: %w: %s", err, commitOut)
    }
    mergeCommit := strings.TrimSpace(string(commitOut))

    // Update the base branch ref.
    cmd = exec.Command("git", "update-ref", "refs/heads/"+base, mergeCommit)
    cmd.Dir = r.Path
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("update-ref: %w", err)
    }

    return &MergeResult{
        MergeCommit: mergeCommit,
        TreeHash:    treeHash,
    }, nil
}

// SquashBranches squashes head into base on a bare repo.
// Creates a single commit on base with all changes from head.
func (r *Repository) SquashBranches(base, head, message string) (*MergeResult, error) {
    // Get the result tree.
    out, err := exec.Command("git", "merge-tree", "--write-tree", base, head).
        CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("merge-tree failed: %w: %s", err, out)
    }
    treeHash := strings.TrimSpace(string(out))

    // Only one parent (base) for squash.
    baseHash, err := r.ShowRefVerify("refs/heads/" + base)
    if err != nil {
        return nil, fmt.Errorf("resolve base: %w", err)
    }

    cmd := exec.Command("git", "commit-tree", treeHash, "-p", baseHash, "-m", message)
    cmd.Dir = r.Path
    cmd.Env = append(cmd.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
    commitOut, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("commit-tree: %w: %s", err, commitOut)
    }
    squashCommit := strings.TrimSpace(string(commitOut))

    cmd = exec.Command("git", "update-ref", "refs/heads/"+base, squashCommit)
    cmd.Dir = r.Path
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("update-ref: %w", err)
    }

    return &MergeResult{
        MergeCommit: squashCommit,
        TreeHash:    treeHash,
    }, nil
}

// RebaseBranches replays head commits onto base on a bare repo.
// Uses git format-patch + git am approach for bare repos.
func (r *Repository) RebaseBranches(base, head string) (*MergeResult, error) {
    mergeBase, err := r.MergeBase(base, head)
    if err != nil {
        return nil, fmt.Errorf("merge base: %w", err)
    }

    // For bare repos, rebase is complex. Use cherry-pick approach:
    // Get commits from merge-base..head, then replay each on base.
    // For simplicity in v1, use the merge-tree approach which gives the same
    // result tree, then fast-forward base to a linear chain.

    // Actually, for bare repo rebase, the cleanest approach is:
    // 1. Get the list of commits to replay
    // 2. For each commit, create a new commit with the same tree delta applied to base
    //
    // Simpler v1: treat rebase as squash with a different message format.
    // TODO: Implement true rebase with individual commit replay in v2.
    return r.SquashBranches(base, head, "")
}
```

**Note:** True rebase on bare repos is complex. For v1, rebase falls back to squash behavior. This can be improved later with `git format-patch` + `git am` in a temporary worktree.

**Verification:** Unit test merging two branches in a bare repo created with `git.Init`.

---

## Phase C: Merge API Endpoint

### Task 3: Add merge endpoint and update PR CRUD with mergeable status

**Files:**
- Modify: `internal/infra/httpapi/api_pulls.go`

**Step 1: Add merge request/response types and the merge handler:**

```go
type mergePullRequestRequest struct {
    MergeMethod string `json:"merge_method"` // "merge", "squash", "rebase"
    Message     string `json:"message,omitempty"`
}

func handleMergePullRequest(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    identity := domain.IdentityFromContext(ctx)
    if identity == nil {
        writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
        return
    }
    store := domain.StoreFromContext(ctx)
    repoName := mux.Vars(r)["repo"]
    number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

    var req mergePullRequestRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
        return
    }

    method := domain.MergeMethod(req.MergeMethod)
    if method != domain.MergeMethodMerge && method != domain.MergeMethodSquash && method != domain.MergeMethodRebase {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "merge_method must be merge, squash, or rebase"})
        return
    }

    repo, err := store.GetRepoByName(ctx, repoName)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
        return
    }

    pr, err := store.GetPullRequestByNumber(ctx, repo.ID, number)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "pull request not found"})
        return
    }

    if pr.Status != domain.PullRequestStatusOpen {
        writeJSON(w, http.StatusConflict, map[string]string{"error": "pull request is not open"})
        return
    }

    // Open the git repo and perform the merge.
    be := domain.BackendFromContext(ctx)
    gitRepo, err := be.OpenRepo(repoName)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to open repository"})
        return
    }

    // Check mergeability.
    mergeable, err := gitRepo.IsMergeable(pr.TargetBranch, pr.SourceBranch)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check mergeability"})
        return
    }
    if !mergeable {
        writeJSON(w, http.StatusConflict, map[string]string{"error": "pull request has conflicts"})
        return
    }

    // Build merge message.
    message := req.Message
    if message == "" {
        message = fmt.Sprintf("Merge pull request #%d from %s\n\n%s", pr.Number, pr.SourceBranch, pr.Title)
    }

    // Execute merge.
    switch method {
    case domain.MergeMethodMerge:
        _, err = gitRepo.MergeBranches(pr.TargetBranch, pr.SourceBranch, message)
    case domain.MergeMethodSquash:
        _, err = gitRepo.SquashBranches(pr.TargetBranch, pr.SourceBranch, message)
    case domain.MergeMethodRebase:
        _, err = gitRepo.RebaseBranches(pr.TargetBranch, pr.SourceBranch)
    }
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "merge failed: " + err.Error()})
        return
    }

    // Update PR status.
    now := time.Now()
    pr.Status = domain.PullRequestStatusMerged
    pr.MergeMethod = &method
    pr.MergedBy = &identity.ID
    pr.MergedAt = &now
    if err := store.UpdatePullRequest(ctx, pr); err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update pull request"})
        return
    }

    // Auto-close referenced issues.
    closeReferencedIssues(ctx, store, repo.ID, pr.Body, identity)

    writeJSON(w, http.StatusOK, prToResponse(ctx, store, pr))

    // Fire webhook.
    if wh, err := webhook.NewPullRequestMergedEvent(ctx, identity, repo, pr); err == nil {
        webhook.SendEvent(ctx, wh) //nolint:errcheck
    }
}
```

**Step 2: Add `mergeable` field to GET PR response:**

Update `pullRequestResponse` to include:
```go
Mergeable *bool `json:"mergeable,omitempty"`
```

In `handleGetPullRequest`, after fetching the PR, if status is open, compute mergeability:
```go
if pr.Status == domain.PullRequestStatusOpen {
    be := domain.BackendFromContext(ctx)
    if gitRepo, err := be.OpenRepo(repoName); err == nil {
        mergeable, err := gitRepo.IsMergeable(pr.TargetBranch, pr.SourceBranch)
        if err == nil {
            resp := prToResponse(ctx, store, pr)
            resp.Mergeable = &mergeable
            writeJSON(w, http.StatusOK, resp)
            return
        }
    }
}
```

**Step 3: Register the merge route in `RegisterPullRequestRoutes`:**

```go
r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}/merge", handleMergePullRequest).Methods("POST")
```

**Verification:** E2E test: create a repo, push two branches, create a PR, merge it.

---

## Phase D: Diff, Commits, and Files Endpoints

### Task 4: Add diff/commits/files API endpoints

**Files:**
- Modify: `internal/infra/httpapi/api_pulls.go`

**Step 1: Add the three diff-related handlers:**

```go
func handlePullRequestDiff(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    store := domain.StoreFromContext(ctx)
    repoName := mux.Vars(r)["repo"]
    number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

    repo, err := store.GetRepoByName(ctx, repoName)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
        return
    }

    pr, err := store.GetPullRequestByNumber(ctx, repo.ID, number)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "pull request not found"})
        return
    }

    be := domain.BackendFromContext(ctx)
    gitRepo, err := be.OpenRepo(repoName)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to open repository"})
        return
    }

    diff, err := gitRepo.DiffBranches(pr.TargetBranch, pr.SourceBranch)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to compute diff"})
        return
    }

    // Return as plain text unified diff.
    w.Header().Set("Content-Type", "text/plain")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(diff.Patch()))
}

type commitResponse struct {
    SHA     string    `json:"sha"`
    Message string    `json:"message"`
    Author  string    `json:"author"`
    Date    time.Time `json:"date"`
}

func handlePullRequestCommits(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    store := domain.StoreFromContext(ctx)
    repoName := mux.Vars(r)["repo"]
    number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

    repo, err := store.GetRepoByName(ctx, repoName)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
        return
    }

    pr, err := store.GetPullRequestByNumber(ctx, repo.ID, number)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "pull request not found"})
        return
    }

    be := domain.BackendFromContext(ctx)
    gitRepo, err := be.OpenRepo(repoName)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to open repository"})
        return
    }

    commits, err := gitRepo.CommitsBetween(pr.TargetBranch, pr.SourceBranch)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list commits"})
        return
    }

    resp := make([]commitResponse, 0, len(commits))
    for _, c := range commits {
        resp = append(resp, commitResponse{
            SHA:     c.ID.String(),
            Message: c.Message,
            Author:  c.Author.Name,
            Date:    c.Author.When,
        })
    }
    writeJSON(w, http.StatusOK, resp)
}

func handlePullRequestFiles(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    store := domain.StoreFromContext(ctx)
    repoName := mux.Vars(r)["repo"]
    number, _ := strconv.ParseInt(mux.Vars(r)["number"], 10, 64)

    repo, err := store.GetRepoByName(ctx, repoName)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
        return
    }

    pr, err := store.GetPullRequestByNumber(ctx, repo.ID, number)
    if err != nil {
        writeJSON(w, http.StatusNotFound, map[string]string{"error": "pull request not found"})
        return
    }

    be := domain.BackendFromContext(ctx)
    gitRepo, err := be.OpenRepo(repoName)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to open repository"})
        return
    }

    files, err := gitRepo.ChangedFilesBetween(pr.TargetBranch, pr.SourceBranch)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list changed files"})
        return
    }

    writeJSON(w, http.StatusOK, files)
}
```

**Step 2: Register the routes:**

```go
r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}/diff", handlePullRequestDiff).Methods("GET")
r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}/commits", handlePullRequestCommits).Methods("GET")
r.HandleFunc("/repos/{repo:.+}/pulls/{number:[0-9]+}/files", handlePullRequestFiles).Methods("GET")
```

**Verification:** E2E test: push branches with known changes, create PR, verify diff/commits/files responses.

---

## Phase E: Commit Keyword Parsing

### Task 5: Add commit keyword parser and auto-close logic

**Files:**
- Create: `internal/infra/git/keywords.go`
- Modify: `internal/infra/httpapi/api_pulls.go` (add helper call)

**Step 1: Create keyword parser:**

```go
package git

import (
    "regexp"
    "strconv"
)

// closingKeywordRe matches "closes #N", "fixes #N", "resolves #N" and variants.
var closingKeywordRe = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+#(\d+)`)

// ParseClosingKeywords extracts issue numbers referenced by closing keywords in text.
func ParseClosingKeywords(text string) []int64 {
    matches := closingKeywordRe.FindAllStringSubmatch(text, -1)
    seen := make(map[int64]bool)
    var nums []int64
    for _, m := range matches {
        if len(m) > 1 {
            n, err := strconv.ParseInt(m[1], 10, 64)
            if err == nil && !seen[n] {
                seen[n] = true
                nums = append(nums, n)
            }
        }
    }
    return nums
}
```

**Step 2: Add `closeReferencedIssues` helper in `api_pulls.go`:**

```go
func closeReferencedIssues(ctx context.Context, store domain.Store, repoID int64, text string, identity *domain.Identity) {
    nums := gitpkg.ParseClosingKeywords(text)
    for _, num := range nums {
        issue, err := store.GetIssueByNumber(ctx, repoID, num)
        if err != nil || issue.Status == domain.IssueStatusClosed {
            continue
        }
        now := time.Now()
        issue.Status = domain.IssueStatusClosed
        issue.Resolution = domain.IssueResolutionFixed
        issue.ClosedAt = &now
        if err := store.UpdateIssue(ctx, issue); err != nil {
            continue
        }

        // Fire issue closed webhook.
        repo, err := store.GetRepoByName(ctx, "") // Need repo — get from context or pass it.
        // Actually, pass repo as parameter to this function.
    }
}
```

Adjust signature to accept `*domain.Repo`:

```go
func closeReferencedIssues(ctx context.Context, store domain.Store, repo *domain.Repo, text string, identity *domain.Identity) {
    nums := gitpkg.ParseClosingKeywords(text)
    for _, num := range nums {
        issue, err := store.GetIssueByNumber(ctx, repo.ID, num)
        if err != nil || issue.Status == domain.IssueStatusClosed {
            continue
        }
        now := time.Now()
        issue.Status = domain.IssueStatusClosed
        issue.Resolution = domain.IssueResolutionFixed
        issue.ClosedAt = &now
        if err := store.UpdateIssue(ctx, issue); err != nil {
            continue
        }
        if wh, err := webhook.NewIssueClosedEvent(ctx, identity, repo, issue, "fixed"); err == nil {
            webhook.SendEvent(ctx, wh) //nolint:errcheck
        }
    }
}
```

Call from `handleMergePullRequest` after merge succeeds, and also parse each commit message in the PR.

**Verification:** Unit test `ParseClosingKeywords` with various patterns. E2E test: create issue, create PR with "fixes #1" in body, merge, verify issue is closed.

---

## Phase F: PR Webhook Events

### Task 6: Add PR webhook event types and constructors

**Files:**
- Modify: `internal/infra/webhook/event.go`
- Create: `internal/infra/webhook/pull_request.go`

**Step 1: Add event constants to `event.go`:**

After `EventIssueComment Event = 10`:

```go
// EventPullRequestOpened fires when a pull request is created.
EventPullRequestOpened Event = 11

// EventPullRequestClosed fires when a pull request is closed without merging.
EventPullRequestClosed Event = 12

// EventPullRequestMerged fires when a pull request is merged.
EventPullRequestMerged Event = 13

// EventPullRequestReview fires when a review is submitted (plan 7c).
EventPullRequestReview Event = 14
```

Also update the `String()` method and any event name mapping.

**Step 2: Create `pull_request.go`:**

```go
package webhook

import (
    "context"
    "time"

    "github.com/Work-Fort/Combine/internal/config"
    "github.com/Work-Fort/Combine/internal/domain"
)

// PullRequestPayload is the PR representation in webhook payloads.
type PullRequestPayload struct {
    Number       int64      `json:"number"`
    Title        string     `json:"title"`
    Body         string     `json:"body"`
    SourceBranch string     `json:"source_branch"`
    TargetBranch string     `json:"target_branch"`
    Status       string     `json:"status"`
    MergeMethod  *string    `json:"merge_method,omitempty"`
    Author       User       `json:"author"`
    MergedBy     *User      `json:"merged_by,omitempty"`
    Assignee     *User      `json:"assignee,omitempty"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
    MergedAt     *time.Time `json:"merged_at,omitempty"`
    ClosedAt     *time.Time `json:"closed_at,omitempty"`
}

type PullRequestOpenedEvent struct {
    Common
    Sender      IdentitySender     `json:"sender"`
    PullRequest PullRequestPayload `json:"pull_request"`
}

type PullRequestClosedEvent struct {
    Common
    Sender      IdentitySender     `json:"sender"`
    PullRequest PullRequestPayload `json:"pull_request"`
}

type PullRequestMergedEvent struct {
    Common
    Sender      IdentitySender     `json:"sender"`
    PullRequest PullRequestPayload `json:"pull_request"`
}

func buildPRCommon(ctx context.Context, repo *domain.Repo, event Event) Common {
    cfg := config.FromContext(ctx)
    c := Common{
        EventType: event,
        Repository: Repository{
            ID:          repo.ID,
            Name:        repo.Name,
            Description: repo.Description,
            ProjectName: repo.ProjectName,
            Private:     repo.Private,
            HTTPURL:     repoURL(cfg.HTTP.PublicURL, repo.Name),
            SSHURL:      repoURL(cfg.SSH.PublicURL, repo.Name),
            CreatedAt:   repo.CreatedAt,
            UpdatedAt:   repo.UpdatedAt,
        },
    }
    c.Repository.DefaultBranch, _ = getDefaultBranch(ctx, repo)
    return c
}

func buildPRPayload(pr *domain.PullRequest) PullRequestPayload {
    p := PullRequestPayload{
        Number:       pr.Number,
        Title:        pr.Title,
        Body:         pr.Body,
        SourceBranch: pr.SourceBranch,
        TargetBranch: pr.TargetBranch,
        Status:       string(pr.Status),
        CreatedAt:    pr.CreatedAt,
        UpdatedAt:    pr.UpdatedAt,
        MergedAt:     pr.MergedAt,
        ClosedAt:     pr.ClosedAt,
    }
    if pr.MergeMethod != nil {
        mm := string(*pr.MergeMethod)
        p.MergeMethod = &mm
    }
    return p
}

func NewPullRequestOpenedEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, pr *domain.PullRequest) (PullRequestOpenedEvent, error) {
    return PullRequestOpenedEvent{
        Common:      buildPRCommon(ctx, repo, EventPullRequestOpened),
        Sender:      identitySender(identity),
        PullRequest: buildPRPayload(pr),
    }, nil
}

func NewPullRequestClosedEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, pr *domain.PullRequest) (PullRequestClosedEvent, error) {
    return PullRequestClosedEvent{
        Common:      buildPRCommon(ctx, repo, EventPullRequestClosed),
        Sender:      identitySender(identity),
        PullRequest: buildPRPayload(pr),
    }, nil
}

func NewPullRequestMergedEvent(ctx context.Context, identity *domain.Identity, repo *domain.Repo, pr *domain.PullRequest) (PullRequestMergedEvent, error) {
    return PullRequestMergedEvent{
        Common:      buildPRCommon(ctx, repo, EventPullRequestMerged),
        Sender:      identitySender(identity),
        PullRequest: buildPRPayload(pr),
    }, nil
}
```

**Step 3: Wire up webhooks in API handlers**

Add webhook calls to:
- `handleCreatePullRequest` — fire `NewPullRequestOpenedEvent`
- `handleUpdatePullRequest` — fire `NewPullRequestClosedEvent` when status changes to closed
- `handleMergePullRequest` — already has `NewPullRequestMergedEvent`

**Verification:** `go build ./...` compiles. E2E test with a webhook endpoint that captures events.

---

## Phase G: Backend OpenRepo Access from HTTP Context

### Task 7: Ensure Backend is accessible from HTTP request context

**Files:**
- Check: `internal/domain/context.go` or equivalent

The merge and diff handlers need to call `be.OpenRepo(repoName)`. Verify that the Backend is available in the request context (similar to how `domain.StoreFromContext(ctx)` works). If not, add `BackendFromContext` and set it in the context middleware.

If Backend is not in context, an alternative is to have the handlers accept a `backend` parameter via closure or struct, similar to how some Go HTTP frameworks work. Follow the existing pattern — if store is in context, add backend the same way.

**Verification:** Merge endpoint works end-to-end.

---

## Summary

After completing Plan 7b:
- `git merge-tree`, `git commit-tree`, `git update-ref` used for bare-repo merges
- Three merge strategies: merge commit, squash, rebase (v1 rebase = squash)
- Diff/commits/files endpoints return branch comparison data
- Mergeability check via `git merge-tree --write-tree`
- Commit keyword parsing auto-closes referenced issues on merge
- PR webhook events: opened, closed, merged
- `GET /pulls/{number}` includes `mergeable` field for open PRs
