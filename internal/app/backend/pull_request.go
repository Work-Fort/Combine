package backend

import (
	"context"
	"fmt"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/git"
	gitmodule "github.com/aymanbagabas/git-module"
)

// IsPullRequestMergeable checks if a PR can be cleanly merged.
func (b *Backend) IsPullRequestMergeable(ctx context.Context, repoName, source, target string) (bool, error) {
	r, err := b.OpenRepo(repoName)
	if err != nil {
		return false, fmt.Errorf("open repo: %w", err)
	}
	return r.IsMergeable(target, source)
}

// MergePullRequest performs the git merge for a PR using the specified strategy.
func (b *Backend) MergePullRequest(ctx context.Context, repoName, source, target string, method domain.MergeMethod, message string) (*git.MergeResult, error) {
	r, err := b.OpenRepo(repoName)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	switch method {
	case domain.MergeMethodMerge:
		return r.MergeBranches(target, source, message)
	case domain.MergeMethodSquash:
		return r.SquashBranches(target, source, message)
	case domain.MergeMethodRebase:
		return r.RebaseBranches(target, source)
	default:
		return nil, fmt.Errorf("unsupported merge method: %s", method)
	}
}

// DiffPullRequest returns the diff between the PR's source and target branches.
func (b *Backend) DiffPullRequest(ctx context.Context, repoName, source, target string) (*git.Diff, error) {
	r, err := b.OpenRepo(repoName)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}
	return r.DiffBranches(target, source)
}

// PullRequestCommits returns commits between the PR's target and source branches.
func (b *Backend) PullRequestCommits(ctx context.Context, repoName, source, target string) ([]*gitmodule.Commit, error) {
	r, err := b.OpenRepo(repoName)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}
	return r.CommitsBetween(target, source)
}

// PullRequestFiles returns the list of changed files in a PR.
func (b *Backend) PullRequestFiles(ctx context.Context, repoName, source, target string) ([]git.ChangedFile, error) {
	r, err := b.OpenRepo(repoName)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}
	return r.ChangedFilesBetween(target, source)
}
