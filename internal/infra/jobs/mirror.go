package jobs

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"charm.land/log/v2"
	"github.com/Work-Fort/Combine/internal/infra/git"
	"github.com/Work-Fort/Combine/internal/app/backend"
	"github.com/Work-Fort/Combine/pkg/config"
	"github.com/Work-Fort/Combine/internal/infra/lfs"
	"github.com/Work-Fort/Combine/internal/infra/sync"
)

func init() {
	Register("mirror-pull", mirrorPull{})
}

type mirrorPull struct{}

// Spec derives the spec used for pull mirrors and implements Runner.
func (m mirrorPull) Spec(ctx context.Context) string {
	cfg := config.FromContext(ctx)
	if cfg.Jobs.MirrorPull != "" {
		return cfg.Jobs.MirrorPull
	}
	return "@every 10m"
}

// Func runs the (pull) mirror job task and implements Runner.
func (m mirrorPull) Func(ctx context.Context) func() {
	cfg := config.FromContext(ctx)
	logger := log.FromContext(ctx).WithPrefix("jobs.mirror")
	b := backend.FromContext(ctx)
	return func() {
		repos, err := b.Repositories(ctx)
		if err != nil {
			logger.Error("error getting repositories", "err", err)
			return
		}

		// Divide the work up among the number of CPUs.
		wq := sync.NewWorkPool(ctx, runtime.GOMAXPROCS(0),
			sync.WithWorkPoolLogger(logger.Errorf),
		)

		logger.Debug("updating mirror repos")
		for _, repo := range repos {
			if repo.Mirror {
				r, err := b.OpenRepo(repo.Name)
				if err != nil {
					logger.Error("error opening repository", "repo", repo.Name, "err", err)
					continue
				}

				name := repo.Name
				wq.Add(name, func() {
					cmds := []string{
						"fetch --prune",         // fetch prune before updating remote
						"remote update --prune", // update remote and prune remote refs
					}

					for _, c := range cmds {
						args := strings.Split(c, " ")
						cmd := git.NewCommand(args...).WithContext(ctx)
						cmd.AddEnvs(
							fmt.Sprintf(`GIT_SSH_COMMAND=ssh -o UserKnownHostsFile="%s" -o StrictHostKeyChecking=no -i "%s"`,
								filepath.Join(cfg.DataPath, "ssh", "known_hosts"),
								cfg.SSH.ClientKeyPath,
							),
						)

						if _, err := cmd.RunInDir(r.Path); err != nil {
							logger.Error("error running git remote update", "repo", name, "err", err)
						}
					}

					if cfg.LFS.Enabled {
						rcfg, err := r.Config()
						if err != nil {
							logger.Error("error getting git config", "repo", name, "err", err)
							return
						}

						lfsEndpoint := rcfg.Section("lfs").Option("url")
						if lfsEndpoint == "" {
							return
						}

						ep, err := lfs.NewEndpoint(lfsEndpoint)
						if err != nil {
							logger.Error("error creating LFS endpoint", "repo", name, "err", err)
							return
						}

						client := lfs.NewClient(ep)
						if client == nil {
							logger.Errorf("failed to create lfs client: unsupported endpoint %s", lfsEndpoint)
							return
						}

						if err := b.StoreRepoMissingLFSObjects(ctx, name, client); err != nil {
							logger.Error("failed to store missing lfs objects", "err", err, "path", r.Path)
							return
						}
					}
				})
			}
		}

		wq.Run()
	}
}
