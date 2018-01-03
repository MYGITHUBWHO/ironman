package git

import (
	"path"
	"strings"

	"github.com/ironman-project/ironman/template/repository"
	"github.com/pkg/errors"
	gogit "gopkg.in/src-d/go-git.v4"
)

var _ *repository.Repository = (*repository.Repository)(nil)

//Repository represents an implementation of a ironman Repository
type Repository struct {
	*repository.BaseRepository
}

//New returns a new instance of the git repository
func New(path string) repository.Repository {
	baseRepository := repository.NewBaseRepository(path)
	return &Repository{baseRepository}
}

//Install installs a template from a git url
func (r *Repository) Install(location string) error {
	templatePath := r.templatePathFromLocation(location)
	gitRepo, err := gogit.NewFilesystemRepository(templatePath)

	if err != nil {
		return errors.Wrapf(err, "Failed to get template repository %s", location)
	}
	err = gitRepo.Clone(&gogit.CloneOptions{
		URL: location,
	})

	if err != nil {
		return errors.Wrapf(err, "Failed to install template  %s", location)
	}
	return nil
}

//Update updates a template from a git repository
func (r *Repository) Update(id string) error {
	templatePath := r.templatePathFromLocation(id)
	gitRepo, err := gogit.NewFilesystemRepository(templatePath)

	if err != nil {
		return errors.Wrapf(err, "Failed to get template repository %s", id)
	}

	err = gitRepo.Pull(&gogit.PullOptions{})

	if gogit.NoErrAlreadyUpToDate != err && err != nil {
		return errors.Wrapf(err, "Failed to Update template  %s", id)
	}
	return nil
}

func (r *Repository) templatePathFromLocation(location string) string {
	templateID := path.Base(strings.TrimSuffix(location, ".git"))
	templatePath := r.TemplatePath(templateID)
	return templatePath
}