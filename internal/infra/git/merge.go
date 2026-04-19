package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	git "github.com/aymanbagabas/git-module"
)

// MergeBase returns the merge-base commit hash between two refs.
func (r *Repository) MergeBase(base, head string) (string, error) {
	return r.Repository.MergeBase(base, head)
}

// DiffBranches returns the diff between two branches (from merge-base to head).
func (r *Repository) DiffBranches(base, head string) (*Diff, error) {
	baseRef := "refs/heads/" + base
	headRef := "refs/heads/" + head
	mergeBase, err := r.MergeBase(baseRef, headRef)
	if err != nil {
		return nil, fmt.Errorf("merge base: %w", err)
	}

	ddiff, err := r.Repository.Diff(headRef, DiffMaxFiles, DiffMaxFileLines, DiffMaxLineChars, git.DiffOptions{
		Base: mergeBase,
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
	baseRef := "refs/heads/" + base
	headRef := "refs/heads/" + head
	mergeBase, err := r.MergeBase(baseRef, headRef)
	if err != nil {
		return nil, fmt.Errorf("merge base: %w", err)
	}

	rev := mergeBase + ".." + headRef
	return r.Repository.Log(rev)
}

// ChangedFile represents a file changed in a diff with stats.
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
func (r *Repository) IsMergeable(ctx context.Context, base, head string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "merge-tree", "--write-tree", base, head)
	cmd.Dir = r.Path
	cmd.Env = append(cmd.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return false, nil
			}
		}
		return false, fmt.Errorf("merge-tree: %w", err)
	}
	return true, nil
}

// MergeResult contains the result of a merge operation.
type MergeResult struct {
	MergeCommit string
	TreeHash    string
}

// MergeBranches performs a merge commit of head into base on a bare repo.
func (r *Repository) MergeBranches(ctx context.Context, base, head, message string) (*MergeResult, error) {
	mtCmd := exec.CommandContext(ctx, "git", "merge-tree", "--write-tree", base, head)
	mtCmd.Dir = r.Path
	out, err := mtCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("merge-tree failed (conflicts?): %w: %s", err, out)
	}
	treeHash := strings.TrimSpace(string(out))

	baseHash, err := r.ShowRefVerify("refs/heads/" + base)
	if err != nil {
		return nil, fmt.Errorf("resolve base: %w", err)
	}
	headHash, err := r.ShowRefVerify("refs/heads/" + head)
	if err != nil {
		return nil, fmt.Errorf("resolve head: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "commit-tree", treeHash,
		"-p", baseHash, "-p", headHash, "-m", message)
	cmd.Dir = r.Path
	cmd.Env = append(cmd.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=Combine",
		"GIT_AUTHOR_EMAIL=combine@localhost",
		"GIT_COMMITTER_NAME=Combine",
		"GIT_COMMITTER_EMAIL=combine@localhost",
	)
	commitOut, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("commit-tree: %w: %s", err, commitOut)
	}
	mergeCommit := strings.TrimSpace(string(commitOut))

	cmd = exec.CommandContext(ctx, "git", "update-ref", "refs/heads/"+base, mergeCommit)
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
func (r *Repository) SquashBranches(ctx context.Context, base, head, message string) (*MergeResult, error) {
	mtCmd := exec.CommandContext(ctx, "git", "merge-tree", "--write-tree", base, head)
	mtCmd.Dir = r.Path
	out, err := mtCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("merge-tree failed: %w: %s", err, out)
	}
	treeHash := strings.TrimSpace(string(out))

	baseHash, err := r.ShowRefVerify("refs/heads/" + base)
	if err != nil {
		return nil, fmt.Errorf("resolve base: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "commit-tree", treeHash, "-p", baseHash, "-m", message)
	cmd.Dir = r.Path
	cmd.Env = append(cmd.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=Combine",
		"GIT_AUTHOR_EMAIL=combine@localhost",
		"GIT_COMMITTER_NAME=Combine",
		"GIT_COMMITTER_EMAIL=combine@localhost",
	)
	commitOut, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("commit-tree: %w: %s", err, commitOut)
	}
	squashCommit := strings.TrimSpace(string(commitOut))

	cmd = exec.CommandContext(ctx, "git", "update-ref", "refs/heads/"+base, squashCommit)
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
// V1: falls back to squash behavior. True rebase with individual commit
// replay can be implemented later with git format-patch + git am.
func (r *Repository) RebaseBranches(ctx context.Context, base, head string) (*MergeResult, error) {
	return r.SquashBranches(ctx, base, head, "")
}
