package backend

import (
	"context"
	"errors"
	"io"
	"path"
	"path/filepath"
	"strconv"

	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/lfs"
	"github.com/Work-Fort/Combine/internal/infra/storage"
)

// StoreRepoMissingLFSObjects stores missing LFS objects for a repository.
func (d *Backend) StoreRepoMissingLFSObjects(ctx context.Context, repoName string, lfsClient lfs.Client) error {
	repo, err := d.store.GetRepoByName(ctx, repoName)
	if err != nil {
		return err
	}

	repoID := strconv.FormatInt(repo.ID, 10)
	lfsRoot := filepath.Join(d.cfg.DataDir, "lfs", repoID)

	// TODO: support S3 storage
	strg := storage.NewLocalStorage(lfsRoot)
	pointerChan := make(chan lfs.PointerBlob)
	errChan := make(chan error, 1)
	r, err := d.OpenRepo(repoName)
	if err != nil {
		return err
	}

	go lfs.SearchPointerBlobs(ctx, r, pointerChan, errChan)

	download := func(pointers []lfs.Pointer) error {
		return lfsClient.Download(ctx, pointers, func(p lfs.Pointer, content io.ReadCloser, objectError error) error {
			if objectError != nil {
				return objectError
			}

			defer content.Close() //nolint: errcheck
			return d.store.Transaction(ctx, func(tx domain.Store) error {
				if err := tx.CreateLFSObject(ctx, repo.ID, p.Oid, p.Size); err != nil {
					return err
				}

				_, err := strg.Put(path.Join("objects", p.RelativePath()), content)
				return err
			})
		})
	}

	var batch []lfs.Pointer
	for pointer := range pointerChan {
		obj, err := d.store.GetLFSObjectByOid(ctx, repo.ID, pointer.Oid)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}

		exist, err := strg.Exists(path.Join("objects", pointer.RelativePath()))
		if err != nil {
			return err
		}

		if exist && (obj == nil || obj.ID == 0) {
			if err := d.store.CreateLFSObject(ctx, repo.ID, pointer.Oid, pointer.Size); err != nil {
				return err
			}
		} else {
			batch = append(batch, pointer.Pointer)
			// Limit batch requests to 20 objects
			if len(batch) >= 20 {
				if err := download(batch); err != nil {
					return err
				}

				batch = nil
			}
		}
	}

	if err, ok := <-errChan; ok {
		return err
	}

	return nil
}
