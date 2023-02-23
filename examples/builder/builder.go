package builder

import (
	"archive/tar"
	"bytes"
	"embed"
	_ "embed"
	"html/template"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/util/slice"
)

//go:embed templates/*.tmpl
var templates embed.FS

type TemplateInput struct {
	TemplateName string

	Kubernetes *KubeOptions
	Docker     *DockerOptions
}

func (in TemplateInput) build(tpl *template.Template) (tf file, readme ReadmeInput, err error) {
	inputs := 0
	if in.Kubernetes != nil {
		inputs = inputs + 1
	}
	if in.Docker != nil {
		inputs = inputs + 1
	}

	if inputs != 1 {
		return file{}, ReadmeInput{}, xerrors.Errorf("expect only 1 input, got %d", inputs)
	}

	switch {
	case in.Kubernetes != nil:
		return buildKube(tpl, *in.Kubernetes)
	case in.Docker != nil:
		return buildDocker(tpl, *in.Docker)
	default:
		return file{}, ReadmeInput{}, xerrors.Errorf("no input provided")
	}
}

type ReadmeInput struct {
	Platform    string
	Name        string
	Description string
	Tags        []string
	Icon        string
}

func buildTemplate(input TemplateInput) ([]byte, error) {
	tpl, err := template.New("").Funcs(template.FuncMap{
		"join":     strings.Join,
		"contains": slice.Contains[string],
		"quote":    func(in string) string { return "\"" + in + "\"" },
	}).ParseFS(templates, "templates/*.tmpl")
	if err != nil {
		return nil, xerrors.Errorf("parse template: %w", err)
	}

	var out bytes.Buffer
	tarWriter := tar.NewWriter(&out)

	tf, readme, err := input.build(tpl)
	if err != nil {
		return nil, err
	}

	// TODO: Should this just be in build?
	md, err := buildReadme(tpl, readme)
	if err != nil {
		return nil, err
	}

	err = writeFiles(tarWriter, tf, md)
	if err != nil {
		return nil, xerrors.Errorf("write tar files: %w", err)
	}

	err = tarWriter.Close()
	if err != nil {
		return nil, xerrors.Errorf("close tar writer: %w", err)
	}
	return out.Bytes(), nil
}

type file struct {
	name    string
	content []byte
}

func buildKube(tpl *template.Template, input KubeOptions) (file, ReadmeInput, error) {
	var out bytes.Buffer
	err := tpl.ExecuteTemplate(&out, "kubernetes", input)
	if err != nil {
		return file{}, ReadmeInput{}, xerrors.Errorf("execute kube template: %w", err)
	}

	return file{
			name:    "main.tf",
			content: out.Bytes(),
		}, ReadmeInput{
			Platform:    "Kubernetes",
			Name:        "Kubernetes based template",
			Description: "Pod based developer workspaces that live in kubernetes.",
			Tags:        []string{"cloud", "kubernetes"},
			Icon:        "/icon/k8s.png",
		}, nil
}

func buildDocker(tpl *template.Template, input DockerOptions) (file, ReadmeInput, error) {
	var out bytes.Buffer
	err := tpl.ExecuteTemplate(&out, "docker", input)
	if err != nil {
		return file{}, ReadmeInput{}, xerrors.Errorf("execute docker template: %w", err)
	}

	return file{
			name:    "main.tf",
			content: out.Bytes(),
		}, ReadmeInput{
			Platform:    "Docker",
			Name:        "Local Docker based template",
			Description: "Local development inside a docker container.",
			Tags:        []string{"local", "docker"},
			Icon:        "/icon/docker.png",
		}, nil
}

func buildReadme(tpl *template.Template, input ReadmeInput) (file, error) {
	var out bytes.Buffer
	err := tpl.ExecuteTemplate(&out, "readme", input)
	if err != nil {
		return file{}, xerrors.Errorf("execute readme template: %w", err)
	}

	return file{
		name:    "README.md",
		content: out.Bytes(),
	}, nil
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

		_, err = w.Write(f.content)
		if err != nil {
			return err
		}
	}

	return nil
}

// Move these to options.go
type DockerOptions struct {
	Image      string
	Env        map[string]string
	HomeVolume bool
	Apps       []string
}
