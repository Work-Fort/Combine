package backend

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/git"
	"github.com/Work-Fort/Combine/internal/infra/hooks"
	"github.com/Work-Fort/Combine/internal/infra/sshutils"
	"github.com/Work-Fort/Combine/internal/infra/webhook"
)

var _ hooks.Hooks = (*Backend)(nil)

// PostReceive is called by the git post-receive hook.
//
// It implements Hooks.
func (d *Backend) PostReceive(_ context.Context, _, _ io.Writer, repo string, args []hooks.HookArg) {
	d.logger.Debug("post-receive hook called", "repo", repo, "args", args)
}

// PreReceive is called by the git pre-receive hook.
//
// It implements Hooks.
func (d *Backend) PreReceive(_ context.Context, _, _ io.Writer, repo string, args []hooks.HookArg) {
	d.logger.Debug("pre-receive hook called", "repo", repo, "args", args)
}

// Update is called by the git update hook.
//
// It implements Hooks.
func (d *Backend) Update(ctx context.Context, _, _ io.Writer, repo string, arg hooks.HookArg) {
	d.logger.Debug("update hook called", "repo", repo, "arg", arg)

	// Find user
	var user *domain.User
	if pubkey := os.Getenv("COMBINE_PUBLIC_KEY"); pubkey != "" {
		pk, _, err := sshutils.ParseAuthorizedKey(pubkey)
		if err != nil {
			d.logger.Error("error parsing public key", "err", err)
			return
		}

		user, err = d.UserByPublicKey(ctx, pk)
		if err != nil {
			d.logger.Error("error finding user from public key", "key", pubkey, "err", err)
			return
		}
	} else if username := os.Getenv("COMBINE_USERNAME"); username != "" {
		var err error
		user, err = d.User(ctx, username)
		if err != nil {
			d.logger.Error("error finding user from username", "username", username, "err", err)
			return
		}
	} else {
		d.logger.Error("error finding user")
		return
	}

	// Get repo
	r, err := d.Repository(ctx, repo)
	if err != nil {
		d.logger.Error("error finding repository", "repo", repo, "err", err)
		return
	}

	// TODO: run this async
	if git.IsZeroHash(arg.OldSha) || git.IsZeroHash(arg.NewSha) {
		wh, err := webhook.NewBranchTagEvent(ctx, user, r, arg.RefName, arg.OldSha, arg.NewSha)
		if err != nil {
			d.logger.Error("error creating branch_tag webhook", "err", err)
		} else if err := webhook.SendEvent(ctx, wh); err != nil {
			d.logger.Error("error sending branch_tag webhook", "err", err)
		}
	}
	wh, err := webhook.NewPushEvent(ctx, user, r, arg.RefName, arg.OldSha, arg.NewSha)
	if err != nil {
		d.logger.Error("error creating push webhook", "err", err)
	} else if err := webhook.SendEvent(ctx, wh); err != nil {
		d.logger.Error("error sending push webhook", "err", err)
	}
}

// PostUpdate is called by the git post-update hook.
//
// It implements Hooks.
func (d *Backend) PostUpdate(ctx context.Context, _, _ io.Writer, repo string, args ...string) {
	d.logger.Debug("post-update hook called", "repo", repo, "args", args)

	var wg sync.WaitGroup

	// Populate last-modified file.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := d.populateLastModified(ctx, repo); err != nil {
			d.logger.Error("error populating last-modified", "repo", repo, "err", err)
			return
		}
	}()

	wg.Wait()
}

func (d *Backend) populateLastModified(ctx context.Context, name string) error {
	_, err := d.Repository(ctx, name)
	if err != nil {
		return err
	}

	r, err := d.OpenRepo(name)
	if err != nil {
		return err
	}

	c, err := r.LatestCommitTime()
	if err != nil {
		return err
	}

	return d.writeLastModified(name, c)
}
