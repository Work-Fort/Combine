package backend

import (
	"github.com/Work-Fort/Combine/internal/domain"
	"github.com/Work-Fort/Combine/internal/infra/git"
)

// LatestFile returns the contents of the latest file at the specified path in
// the repository and its file path.
func LatestFile(d *Backend, r *domain.Repo, ref *git.Reference, pattern string) (string, string, error) {
	repo, err := d.OpenRepo(r.Name)
	if err != nil {
		return "", "", err
	}
	return git.LatestFile(repo, ref, pattern)
}

// Readme returns the repository's README.
func Readme(d *Backend, r *domain.Repo, ref *git.Reference) (readme, path string, err error) {
	pattern := "[rR][eE][aA][dD][mM][eE]*"
	readme, path, err = LatestFile(d, r, ref, pattern)
	return readme, path, err
}
