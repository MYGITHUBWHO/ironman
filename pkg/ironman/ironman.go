package ironman

import (
	"bytes"
	"context"
	"io"

	"log"
	"os"
	"path/filepath"
	gtemplate "text/template"

	"github.com/ironman-project/ironman/pkg/template"
	"github.com/ironman-project/ironman/pkg/template/index"
	"github.com/ironman-project/ironman/pkg/template/values"

	"github.com/ironman-project/ironman/pkg/template/validator"

	"github.com/ironman-project/ironman/pkg/template/index/storm"
	"github.com/ironman-project/ironman/pkg/template/manager"
	"github.com/ironman-project/ironman/pkg/template/manager/git"
	"github.com/ironman-project/ironman/pkg/template/model"
	"github.com/pkg/errors"
)

const (
	indexName          = "templates.index"
	templatesDirectory = "templates"
	generatorsPath     = "generators"
)

const validatoinTemplateText = ``

//Ironman is the one administering the local
type Ironman struct {
	manager                manager.Manager
	modelReader            model.Reader
	index                  index.Index
	home                   string
	validators             []validator.Validator
	output                 io.Writer
	validationTempl        *gtemplate.Template
	validationTemplateText string
}

//New returns a new instance of ironman
func New(home string, options ...Option) *Ironman {

	ir := &Ironman{home: home, output: os.Stdout}

	for _, option := range options {
		option(ir)
	}
	var err error
	ir.validationTempl, err = gtemplate.New("validationTemplate").Parse(validatoinTemplateText)
	if err != nil {
		log.Fatalf("failed to initialize validation errors template %s", err)
	}

	if ir.manager == nil {
		manager := git.New(home, templatesDirectory, git.SetOutput(ir.output))
		ir.manager = manager
	}

	if ir.index == nil {
		indexPath := filepath.Join(home, indexName)
		index := storm.New(storm.DefaultDBFactory(indexPath))
		ir.index = index
	}

	if ir.modelReader == nil {
		decoder := model.NewDecoder(model.DecoderTypeYAML)
		modelReader := model.NewFSReader([]string{".git"}, model.MetadataFileExtensionYAML, decoder, generatorsPath)
		ir.modelReader = modelReader
	}

	if ir.validators == nil {
		ir.validators = []validator.Validator{}
	}

	return ir
}

//Install installs a new template based on a template locator
func (i *Ironman) Install(templateLocator string) error {

	templateDirectory, err := i.manager.Install(templateLocator)

	if err != nil {
		return err
	}

	templatePath := i.manager.TemplateLocation(templateDirectory)

	templateModel, err := i.modelReader.Read(templatePath)

	if err != nil {
		//rollback manager installation
		_ = i.manager.Uninstall(templateDirectory)
		return errors.Wrap(err, "failed to read template model")
	}

	//validate model
	for _, validator := range i.validators {
		valid, validationErr, err := validator.Validate(templateModel)

		if err != nil {
			return errors.Wrap(err, "failed to validate model")
		}

		if !valid {
			var validationErrBuffer bytes.Buffer
			err := i.validationTempl.Execute(&validationErrBuffer, validationErr)

			if err != nil {
				return errors.Wrap(err, "failed to create validation error message")
			}

			return errors.New(validationErrBuffer.String())
		}
	}

	//Set the installation type
	templateModel.SourceType = model.SourceTypeURL
	_, err = i.index.Index(templateModel)

	if err != nil {
		//rollback manager installation
		_ = i.manager.Uninstall(templateDirectory)
		return err
	}

	return nil
}

//Link Creates a symlink to the ironman repository from any path in the filesystem
func (i *Ironman) Link(templatePath, templateID string) error {

	linkPath, err := i.manager.Link(templatePath, templateID)

	if err != nil {
		return err
	}

	templateModel, err := i.modelReader.Read(linkPath)

	if err != nil {
		_ = i.manager.Unlink(templateID)
		return err
	}

	templateModel.ID = templateID
	templateModel.SourceType = model.SourceTypeLink
	_, err = i.index.Index(templateModel)

	if err != nil {
		_ = i.manager.Unlink(templateID)
		return err
	}

	return nil
}

//List returns a list of all the installed ironman templates
func (i *Ironman) List() ([]*model.Template, error) {
	results, err := i.index.List()
	if err != nil {
		return nil, err
	}

	return results, nil
}

//Uninstall uninstalls an ironman template
func (i *Ironman) Uninstall(templateID string) error {

	exists, err := i.index.Exists(templateID)

	if err != nil {
		return errors.Wrapf(err, "failed to validate if template exists %s", templateID)
	}

	if !exists {
		return errors.Errorf("template %s is not installed", templateID)
	}

	model, err := i.index.FindTemplateByID(templateID)

	if err != nil {
		return errors.Wrapf(err, "failed to get template model %s", templateID)
	}

	err = i.manager.Uninstall(model.DirectoryName)

	if err != nil {
		return err
	}

	_, err = i.index.Delete(templateID)

	if err != nil {
		return err
	}

	return nil
}

//Unlink unlinks a previously linked ironman template
func (i *Ironman) Unlink(templateID string) error {

	err := i.manager.Unlink(templateID)

	if err != nil {
		return err
	}

	_, err = i.index.Delete(templateID)

	if err != nil {
		return err
	}

	return nil
}

//Update updates an iroman template
func (i *Ironman) Update(templateID string) error {
	exists, err := i.index.Exists(templateID)

	if err != nil {
		return errors.Wrapf(err, "failed to validate if template exists %s", templateID)
	}

	if !exists {
		return errors.Errorf("template '%s' is not installed", templateID)
	}

	model, err := i.index.FindTemplateByID(templateID)

	if err != nil {
		return errors.Wrapf(err, "failed to get template model %s", templateID)
	}

	err = i.manager.Update(model.DirectoryName)

	if err != nil {
		return err
	}

	return nil
}

//Create creates a new template based on the name and path
func (i *Ironman) Create(templatePath string) error {
	err := template.Create(templatePath, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to create template %s", templatePath)
	}

	return nil
}

//Generate generates a new file or directory based on a generator
func (i *Ironman) Generate(context context.Context, templateID string, generatorID string, generationPath string, values values.Values, force bool) error {
	//First validate if template exists
	exists, err := i.index.Exists(templateID)

	if err != nil {
		return errors.Wrapf(err, "failed to validate if template exists %s", templateID)
	}

	if !exists {
		return errors.Errorf("template '%s' is not installed", templateID)
	}

	templateModel, err := i.index.FindTemplateByID(templateID)
	if err != nil {
		return errors.Wrapf(err, "could not find template by ID %s", templateID)
	}

	//Update metadata of the template automatically if the template type is a link
	if templateModel.SourceType == model.SourceTypeLink {
		templatePath := i.manager.TemplateLocation(templateModel.DirectoryName)
		templateModel, err = i.modelReader.Read(templatePath)
		if err != nil {
			return errors.Wrapf(err, "failed to update metadata for template %s", templateID)
		}
		//reset the template ID  and SourceType since a linked template has a custom ID and SourceType are not the one defined in metadata
		templateModel.ID = templateID
		templateModel.SourceType = model.SourceTypeLink
		err = i.index.Update(templateModel)

		if err != nil {
			return errors.Wrapf(err, "Failed to update metadata for template %s", templateID)
		}
	}

	//Get the generator after all the valitations to the template have been made
	genteratorModel := templateModel.Generator(generatorID)

	if genteratorModel == nil {
		return errors.Errorf("generator %s does not exists", generatorID)
	}

	absGenerationPath, err := filepath.Abs(generationPath)

	if err != nil {
		return errors.Wrapf(err, "failed to get absolute path for generation path %s", generationPath)
	}

	if genteratorModel.TType == model.GeneratorTypeFile {

		baseDir := filepath.Dir(absGenerationPath)

		if _, err := os.Stat(baseDir); os.IsNotExist(err) {
			return errors.Errorf("directory %s does not exists", filepath.Dir(generationPath))
		}

		fileName := filepath.Base(absGenerationPath)
		filePath := filepath.Join(baseDir, genteratorModel.FileTypeOptions.FileGenerationRelativePath, fileName)

		if _, err := os.Stat(filePath); err == nil && !force {
			return errors.Errorf("file already exists %s ", filePath)
		}

	} else {
		//If template exists validate generation directory
		err = os.Mkdir(absGenerationPath, os.ModePerm)

		if os.IsPermission(err) {
			return errors.Wrapf(err, "failed to create generation path %s", absGenerationPath)
		} else if os.IsExist(err) && !force {
			empty, err := isDirEmpty(absGenerationPath)

			if err != nil {
				return errors.Wrapf(err, "failed to validate if generation path is empty %s", err)
			}

			if !empty {
				return errors.Errorf("Generation path is not empty %s", absGenerationPath)
			}

		}
	}

	generatorPath := filepath.Join(i.home, templatesDirectory, templateModel.DirectoryName, generatorsPath, genteratorModel.DirectoryName)

	data := template.GeneratorData{
		Template:  templateModel,
		Generator: genteratorModel,
		Values:    values,
	}

	generator := template.NewGenerator(
		generatorPath,
		absGenerationPath,
		data,
		template.SetGeneratorOutput(i.output),
	)

	if err := generator.Generate(context); err != nil {
		return err
	}

	return nil
}

func isDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}

//EnsureIronmanHome ensures the ironman home directory
func (i *Ironman) EnsureIronmanHome() error {
	if _, err := os.Stat(i.home); os.IsNotExist(err) {
		err := os.Mkdir(i.home, os.ModePerm)
		if err != nil {
			return errors.Wrapf(err, "failed to initialize ironman home '%s'", i.home)
		}

		err = os.Mkdir(filepath.Join(i.home, templatesDirectory), os.ModePerm)

		if err != nil {
			return errors.Wrapf(err, "failed to initialize ironman home '%s'", i.home)
		}
	}
	return nil
}

//EnsureIronmanHome inits the ironman home directory
func EnsureIronmanHome(ironmanHome string) error {
	if _, err := os.Stat(ironmanHome); os.IsNotExist(err) {
		err := os.Mkdir(ironmanHome, os.ModePerm)
		if err != nil {
			return errors.Wrap(err, "failed to initialize ironman home")
		}

		err = os.Mkdir(filepath.Join(ironmanHome, templatesDirectory), os.ModePerm)

		if err != nil {
			return errors.Wrap(err, "failed to initialize ironman home")
		}
	}
	return nil
}
