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

	Kubernetes KubeOptions
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

	tf, readme, err := buildKube(tpl, input.Kubernetes)
	if err != nil {
		return nil, err
	}

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
