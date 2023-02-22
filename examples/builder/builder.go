package builder

import (
	"archive/tar"
	"bytes"
	"embed"
	_ "embed"
	"golang.org/x/xerrors"
	"html/template"
	"strings"
	"time"
)

//go:embed templates/*.tmpl
var templates embed.FS

func buildTemplate(input ContainerInput) (file, error) {
	tpl, err := template.ParseFS(templates, "templates/*.tmpl")
	if err != nil {
		return file{}, xerrors.Errorf("parse template: %w", err)
	}

	tpl = tpl.Funcs(template.FuncMap{
		"join": strings.Join,
	})

	var buf bytes.Buffer
	// TODO: Change the name based on the input
	err = tpl.ExecuteTemplate(&buf, "docker", input)
	if err != nil {
		return file{}, xerrors.Errorf("execute template: %w", err)
	}

	return file{
		name:    "main.tf",
		content: buf.Bytes(),
	}, nil
}

type file struct {
	name    string
	content []byte
}

func writeFiles(w *tar.Writer, files ...file) error {
	for _, f := range files {
		err := w.WriteHeader(&tar.Header{
			Typeflag: 0,
			Name:     f.name,
			Size:     int64(len(f.content)),
			Mode:     0644,
			ModTime:  time.Now(),
		})

		if err != nil {
			return err
		}
	}

	return nil
}

func BuildReadme() {

}

func dockerTemplate() (*template.Template, error) {
	return template.ParseFS(templates, "templates/docker.tpl")
}

type ContainerInput struct {
	DockerImage string `json:"docker-image"`
}

type KubernetesInput struct {
	ContainerInput
}

func BuildDocker(input interface{}) {

}

func build(input interface{}) (string, error) {
	tpl, err := dockerTemplate()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = tpl.Execute(&buf, input)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
