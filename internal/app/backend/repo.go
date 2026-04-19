package backend

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/git"
	"github.com/Work-Fort/Combine/internal/infra/hooks"
	"github.com/Work-Fort/Combine/internal/infra/lfs"
	"github.com/Work-Fort/Combine/internal/infra/storage"
	"github.com/Work-Fort/Combine/internal/infra/task"
	"github.com/Work-Fort/Combine/internal/infra/utils"
	"github.com/Work-Fort/Combine/internal/infra/webhook"
)

// CreateRepository creates a new repository.
func (d *Backend) CreateRepository(ctx context.Context, name string, identity *domain.Identity, opts domain.RepoOptions) (*domain.Repo, error) {
	name = utils.SanitizeRepo(name)
	if err := utils.ValidateRepo(name); err != nil {
		return nil, err
	}

	rp := d.repoPath(name)

	var identityID *string
	if identity != nil {
		id := identity.ID
		identityID = &id
	}

	if err := d.store.Transaction(ctx, func(tx domain.Store) error {
		repo := &domain.Repo{
			Name:        name,
			ProjectName: opts.ProjectName,
			Description: opts.Description,
			Private:     opts.Private,
			Hidden:      opts.Hidden,
			Mirror:      opts.Mirror,
			IdentityID:  identityID,
		}
		if err := tx.CreateRepo(ctx, repo); err != nil {
			return err
		}

		_, err := git.Init(rp, true)
		if err != nil {
			d.logger.Debug("failed to create repository", "err", err)
			return err
		}

		if err := os.WriteFile(filepath.Join(rp, "description"), []byte(opts.Description), fs.ModePerm); err != nil {
			d.logger.Error("failed to write description", "repo", name, "err", err)
			return err
		}

		return hooks.GenerateHooksWithPaths(ctx, d.cfg.DataDir, name)
	}); err != nil {
		d.logger.Debug("failed to create repository in database", "err", err)
		if errors.Is(err, domain.ErrAlreadyExists) {
			return nil, domain.ErrRepoExist
		}

		return nil, err
	}

	return d.Repository(ctx, name)
}

// ImportRepository imports a repository from remote.
// XXX: This a expensive operation and should be run in a goroutine.
func (d *Backend) ImportRepository(_ context.Context, name string, identity *domain.Identity, remote string, opts domain.RepoOptions) (*domain.Repo, error) {
	name = utils.SanitizeRepo(name)
	if err := utils.ValidateRepo(name); err != nil {
		return nil, err
	}

	rp := d.repoPath(name)

	tid := "import:" + name
	if d.manager.Exists(tid) {
		return nil, task.ErrAlreadyStarted
	}

	if _, err := os.Stat(rp); err == nil || os.IsExist(err) {
		return nil, domain.ErrRepoExist
	}

	done := make(chan error, 1)
	repoc := make(chan *domain.Repo, 1)
	d.logger.Info("importing repository", "name", name, "remote", remote, "path", rp)
	d.manager.Add(tid, func(ctx context.Context) (err error) {
		ctx = domain.WithIdentityContext(ctx, identity)

		copts := git.CloneOptions{
			Bare:   true,
			Mirror: opts.Mirror,
			Quiet:  true,
			CommandOptions: git.CommandOptions{
				Timeout: -1,
				Context: ctx,
				Envs: []string{
					fmt.Sprintf(`GIT_SSH_COMMAND=ssh -o UserKnownHostsFile="%s" -o StrictHostKeyChecking=no -i "%s"`,
						d.cfg.SSHKnownHostsPath,
						d.cfg.SSHClientKeyPath,
					),
				},
			},
		}

		if err := git.Clone(remote, rp, copts); err != nil {
			d.logger.Error("failed to clone repository", "err", err, "mirror", opts.Mirror, "remote", remote, "path", rp)
			// Cleanup the mess!
			if rerr := os.RemoveAll(rp); rerr != nil {
				err = errors.Join(err, rerr)
			}

			return err
		}

		r, err := d.CreateRepository(ctx, name, identity, opts)
		if err != nil {
			d.logger.Error("failed to create repository", "err", err, "name", name)
			return err
		}

		defer func() {
			if err != nil {
				if rerr := d.DeleteRepository(ctx, name); rerr != nil {
					d.logger.Error("failed to delete repository", "err", rerr, "name", name)
				}
			}
		}()

		rr, err := d.OpenRepo(name)
		if err != nil {
			d.logger.Error("failed to open repository", "err", err, "path", rp)
			return err
		}

		repoc <- r

		rcfg, err := rr.Config()
		if err != nil {
			d.logger.Error("failed to get repository config", "err", err, "path", rp)
			return err
		}

		endpoint := remote
		if opts.LFSEndpoint != "" {
			endpoint = opts.LFSEndpoint
		}

		rcfg.Section("lfs").SetOption("url", endpoint)

		if err := rr.SetConfig(rcfg); err != nil {
			d.logger.Error("failed to set repository config", "err", err, "path", rp)
			return err
		}

		ep, err := lfs.NewEndpoint(endpoint)
		if err != nil {
			d.logger.Error("failed to create lfs endpoint", "err", err, "path", rp)
			return err
		}

		client := lfs.NewClient(ep)
		if client == nil {
			d.logger.Warn("failed to create lfs client: unsupported endpoint", "endpoint", endpoint)
			return nil
		}

		if err := d.StoreRepoMissingLFSObjects(ctx, name, client); err != nil {
			d.logger.Error("failed to store missing lfs objects", "err", err, "path", rp)
			return err
		}

		return nil
	})

	go func() {
		d.logger.Info("running import", "name", name)
		d.manager.Run(tid, done)
	}()

	return <-repoc, <-done
}

// DeleteRepository deletes a repository.
func (d *Backend) DeleteRepository(ctx context.Context, name string) error {
	name = utils.SanitizeRepo(name)
	rp := d.repoPath(name)

	identity := domain.IdentityFromContext(ctx)
	r, err := d.Repository(ctx, name)
	if err != nil {
		return err
	}

	// We create the webhook event before deleting the repository so we can
	// send the event after deleting the repository.
	wh, err := webhook.NewRepositoryEvent(ctx, identity, r, webhook.RepositoryEventActionDelete)
	if err != nil {
		return err
	}

	if err := d.store.Transaction(ctx, func(tx domain.Store) error {
		// Delete repo from cache
		defer d.cache.Delete(name)

		repo, dberr := tx.GetRepoByName(ctx, name)
		_, ferr := os.Stat(rp)
		if dberr != nil && ferr != nil {
			return domain.ErrRepoNotFound
		}

		// If the repo is not in the database but the directory exists, remove it
		if dberr != nil && ferr == nil {
			return os.RemoveAll(rp)
		} else if dberr != nil {
			return dberr
		}

		repoID := strconv.FormatInt(repo.ID, 10)
		strg := storage.NewLocalStorage(filepath.Join(d.cfg.DataDir, "lfs", repoID))
		objs, err := tx.ListLFSObjectsByName(ctx, name)
		if err != nil {
			return err
		}

		for _, obj := range objs {
			p := lfs.Pointer{
				Oid:  obj.Oid,
				Size: obj.Size,
			}

			d.logger.Debug("deleting lfs object", "repo", name, "oid", obj.Oid)
			if err := strg.Delete(path.Join("objects", p.RelativePath())); err != nil {
				d.logger.Error("failed to delete lfs object", "repo", name, "err", err, "oid", obj.Oid)
			}
		}

		if err := tx.DeleteRepoByName(ctx, name); err != nil {
			return err
		}

		return os.RemoveAll(rp)
	}); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrRepoNotFound
		}

		return err
	}

	return webhook.SendEvent(ctx, wh)
}

// DeleteIdentityRepositories deletes all repositories owned by an identity.
func (d *Backend) DeleteIdentityRepositories(ctx context.Context, identityID string) error {
	repos, err := d.store.ListReposByIdentityID(ctx, identityID)
	if err != nil {
		return err
	}

	for _, repo := range repos {
		if err := d.DeleteRepository(ctx, repo.Name); err != nil {
			return err
		}
	}

	return nil
}

// RenameRepository renames a repository.
func (d *Backend) RenameRepository(ctx context.Context, oldName, newName string) error {
	oldName = utils.SanitizeRepo(oldName)
	if err := utils.ValidateRepo(oldName); err != nil {
		return err
	}

	newName = utils.SanitizeRepo(newName)
	if err := utils.ValidateRepo(newName); err != nil {
		return err
	}

	if oldName == newName {
		return nil
	}

	op := d.repoPath(oldName)
	np := d.repoPath(newName)
	if _, err := os.Stat(op); err != nil {
		return domain.ErrRepoNotFound
	}

	if _, err := os.Stat(np); err == nil {
		return domain.ErrRepoExist
	}

	if err := d.store.Transaction(ctx, func(tx domain.Store) error {
		// Delete cache
		defer d.cache.Delete(oldName)

		// Get old repo, update name
		repo, err := tx.GetRepoByName(ctx, oldName)
		if err != nil {
			return err
		}
		repo.Name = newName
		if err := tx.UpdateRepo(ctx, repo); err != nil {
			return err
		}

		// Make sure the new repository parent directory exists.
		if err := os.MkdirAll(filepath.Dir(np), os.ModePerm); err != nil {
			return err
		}

		return os.Rename(op, np)
	}); err != nil {
		return err
	}

	identity := domain.IdentityFromContext(ctx)
	repo, err := d.Repository(ctx, newName)
	if err != nil {
		return err
	}

	wh, err := webhook.NewRepositoryEvent(ctx, identity, repo, webhook.RepositoryEventActionRename)
	if err != nil {
		return err
	}

	return webhook.SendEvent(ctx, wh)
}

// Repositories returns a list of all repositories.
func (d *Backend) Repositories(ctx context.Context) ([]*domain.Repo, error) {
	ms, err := d.store.ListRepos(ctx)
	if err != nil {
		return nil, err
	}

	for _, m := range ms {
		d.cache.Set(m.Name, m)
	}

	return ms, nil
}

// Repository returns a repository by name.
func (d *Backend) Repository(ctx context.Context, name string) (*domain.Repo, error) {
	name = utils.SanitizeRepo(name)

	if r, ok := d.cache.Get(name); ok && r != nil {
		return r, nil
	}

	rp := d.repoPath(name)
	if _, err := os.Stat(rp); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			d.logger.Errorf("failed to stat repository path: %v", err)
		}
		return nil, domain.ErrRepoNotFound
	}

	m, err := d.store.GetRepoByName(ctx, name)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrRepoNotFound
		}
		return nil, err
	}

	// Compute UpdatedAt from filesystem/git data
	m.UpdatedAt = d.computeUpdatedAt(name, m.UpdatedAt)

	// Add to cache
	d.cache.Set(name, m)

	return m, nil
}

// OpenRepo opens the git repository for the given repo name.
func (d *Backend) OpenRepo(name string) (*git.Repository, error) {
	return git.Open(d.repoPath(name))
}

// Description returns the description of a repository.
func (d *Backend) Description(ctx context.Context, name string) (string, error) {
	name = utils.SanitizeRepo(name)
	repo, err := d.store.GetRepoByName(ctx, name)
	if err != nil {
		return "", err
	}
	return repo.Description, nil
}

// IsMirror returns true if the repository is a mirror.
func (d *Backend) IsMirror(ctx context.Context, name string) (bool, error) {
	name = utils.SanitizeRepo(name)
	repo, err := d.store.GetRepoByName(ctx, name)
	if err != nil {
		return false, err
	}
	return repo.Mirror, nil
}

// IsPrivate returns true if the repository is private.
func (d *Backend) IsPrivate(ctx context.Context, name string) (bool, error) {
	name = utils.SanitizeRepo(name)
	repo, err := d.store.GetRepoByName(ctx, name)
	if err != nil {
		return false, err
	}
	return repo.Private, nil
}

// IsHidden returns true if the repository is hidden.
func (d *Backend) IsHidden(ctx context.Context, name string) (bool, error) {
	name = utils.SanitizeRepo(name)
	repo, err := d.store.GetRepoByName(ctx, name)
	if err != nil {
		return false, err
	}
	return repo.Hidden, nil
}

// ProjectName returns the project name of a repository.
func (d *Backend) ProjectName(ctx context.Context, name string) (string, error) {
	name = utils.SanitizeRepo(name)
	repo, err := d.store.GetRepoByName(ctx, name)
	if err != nil {
		return "", err
	}
	return repo.ProjectName, nil
}

// SetHidden sets the hidden flag of a repository.
func (d *Backend) SetHidden(ctx context.Context, name string, hidden bool) error {
	name = utils.SanitizeRepo(name)
	d.cache.Delete(name)

	return d.store.Transaction(ctx, func(tx domain.Store) error {
		repo, err := tx.GetRepoByName(ctx, name)
		if err != nil {
			return err
		}
		repo.Hidden = hidden
		return tx.UpdateRepo(ctx, repo)
	})
}

// SetDescription sets the description of a repository.
func (d *Backend) SetDescription(ctx context.Context, name, desc string) error {
	name = utils.SanitizeRepo(name)
	desc = utils.Sanitize(desc)
	rp := d.repoPath(name)
	d.cache.Delete(name)

	return d.store.Transaction(ctx, func(tx domain.Store) error {
		if err := os.WriteFile(filepath.Join(rp, "description"), []byte(desc), fs.ModePerm); err != nil {
			d.logger.Error("failed to write description", "repo", name, "err", err)
			return err
		}

		repo, err := tx.GetRepoByName(ctx, name)
		if err != nil {
			return err
		}
		repo.Description = desc
		return tx.UpdateRepo(ctx, repo)
	})
}

// SetPrivate sets the private flag of a repository.
func (d *Backend) SetPrivate(ctx context.Context, name string, private bool) error {
	name = utils.SanitizeRepo(name)
	d.cache.Delete(name)

	if err := d.store.Transaction(ctx, func(tx domain.Store) error {
		repo, err := tx.GetRepoByName(ctx, name)
		if err != nil {
			return err
		}
		repo.Private = private
		return tx.UpdateRepo(ctx, repo)
	}); err != nil {
		return err
	}

	identity := domain.IdentityFromContext(ctx)
	repo, err := d.Repository(ctx, name)
	if err != nil {
		return err
	}

	if repo.Private != !private {
		wh, err := webhook.NewRepositoryEvent(ctx, identity, repo, webhook.RepositoryEventActionVisibilityChange)
		if err != nil {
			return err
		}

		if err := webhook.SendEvent(ctx, wh); err != nil {
			return err
		}
	}

	return nil
}

// SetProjectName sets the project name of a repository.
func (d *Backend) SetProjectName(ctx context.Context, repoName, name string) error {
	repoName = utils.SanitizeRepo(repoName)
	name = utils.Sanitize(name)
	d.cache.Delete(repoName)

	return d.store.Transaction(ctx, func(tx domain.Store) error {
		repo, err := tx.GetRepoByName(ctx, repoName)
		if err != nil {
			return err
		}
		repo.ProjectName = name
		return tx.UpdateRepo(ctx, repo)
	})
}

// repoPath returns the path to a repository.
func (d *Backend) repoPath(name string) string {
	name = utils.SanitizeRepo(name)
	rn := strings.ReplaceAll(name, "/", string(os.PathSeparator))
	return filepath.Join(d.cfg.RepoDir, rn+".git")
}

// computeUpdatedAt returns the best "updated at" time for a repository.
// It checks last-modified file, then latest commit, then falls back to the DB value.
func (d *Backend) computeUpdatedAt(name string, fallback time.Time) time.Time {
	rp := d.repoPath(name)

	// Try to read the last modified time from the info directory.
	if t, err := readOneline(filepath.Join(rp, "info", "last-modified")); err == nil {
		if t, err := time.Parse(time.RFC3339, t); err == nil {
			return t
		}
	}

	rr, err := git.Open(rp)
	if err == nil {
		t, err := rr.LatestCommitTime()
		if err == nil {
			return t
		}
	}

	return fallback
}

// writeLastModified writes the last-modified time to the repository's info directory.
func (d *Backend) writeLastModified(name string, t time.Time) error {
	rp := d.repoPath(name)
	fp := filepath.Join(rp, "info", "last-modified")
	if err := os.MkdirAll(filepath.Dir(fp), os.ModePerm); err != nil {
		return err
	}

	return os.WriteFile(fp, []byte(t.Format(time.RFC3339)), os.ModePerm) //nolint:gosec
}

func readOneline(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer f.Close() //nolint: errcheck
	s := bufio.NewScanner(f)
	s.Scan()
	return s.Text(), s.Err()
}
