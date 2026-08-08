package gen

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

//go:embed runtime/*.odin
var runtimeFS embed.FS

// --- template-facing helpers on IR types ---

// Tag renders the Odin struct tag, empty when the default field name works.
func (f Field) Tag() string {
	if f.WireName == f.OdinName && !f.OmitEmpty {
		return ""
	}
	t := f.WireName
	if f.OmitEmpty {
		t += ",omitempty"
	}
	return `json:"` + t + `"`
}

func (o *Operation) HasQuery() bool {
	return len(o.RequiredQuery) > 0 || len(o.OptQuery) > 0
}

func (o *Operation) IsNone() bool  { return o.ResultKind == ResultNone }
func (o *Operation) IsTyped() bool { return o.ResultKind == ResultTyped }
func (o *Operation) IsRaw() bool   { return o.ResultKind == ResultRaw }

func (o *Operation) ReturnSig() string {
	switch o.ResultKind {
	case ResultTyped:
		return fmt.Sprintf("(result: %s, err: Error)", o.ResultType)
	case ResultRaw:
		return "(result: json.Value, err: Error)"
	default:
		return "(err: Error)"
	}
}

func (o *Operation) PathArgNames() string {
	names := make([]string, len(o.PathArgs))
	for i, p := range o.PathArgs {
		names[i] = p.OdinName
	}
	return strings.Join(names, ", ")
}

// group-level facts the template needs for imports
type fileGroup struct {
	*Group
	Package  string
	Source   string
	UsesFmt  bool
	UsesJSON bool
	// Options structs owned by this file's operations.
	Options []*Model
}

var funcs = template.FuncMap{
	// comment renders doc lines as an Odin comment block at an indent level.
	"comment": func(indent int, lines []string) string {
		if len(lines) == 0 {
			return ""
		}
		pad := strings.Repeat("\t", indent)
		var b strings.Builder
		for _, l := range lines {
			if l == "" {
				b.WriteString(pad + "//\n")
			} else {
				b.WriteString(pad + "// " + l + "\n")
			}
		}
		return b.String()
	},
	"qbAdd": func(p Param) string {
		return "_qb_add_" + p.OdinType
	},
	"qbMaybe": func(p Param) string {
		return "_qb_maybe_" + p.OdinType
	},
}

// Emit writes the generated SDK package into outDir: models.odin, one
// api_<tag>.odin per tag, and the static runtime files.
func Emit(pkg *Package, source, outDir, baseURL string) error {
	tmpl, err := template.New("").Funcs(funcs).ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	optionsNames := map[string]bool{}
	for _, g := range pkg.Groups {
		for _, op := range g.Ops {
			if op.Options != nil {
				optionsNames[op.Options.Name] = true
			}
		}
	}
	var models []*Model
	for _, m := range pkg.Models {
		if !optionsNames[m.Name] {
			models = append(models, m)
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })

	render := func(name, file string, data any) error {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
			return fmt.Errorf("template %s: %w", name, err)
		}
		return os.WriteFile(filepath.Join(outDir, file), buf.Bytes(), 0o644)
	}

	err = render("models.odin.tmpl", "models.odin", map[string]any{
		"Package": pkg.Name,
		"Source":  source,
		"Models":  models,
		"Enums":   pkg.Enums,
	})
	if err != nil {
		return err
	}

	for _, g := range pkg.Groups {
		fg := &fileGroup{Group: g, Package: pkg.Name, Source: source}
		for _, op := range g.Ops {
			if len(op.PathArgs) > 0 {
				fg.UsesFmt = true
			}
			if op.ResultKind == ResultRaw {
				fg.UsesJSON = true
			}
			if op.Options != nil {
				fg.Options = append(fg.Options, op.Options)
			}
		}
		if err := render("api.odin.tmpl", g.FileName, fg); err != nil {
			return err
		}
	}

	// Copy the static runtime, substituting the package name.
	entries, err := runtimeFS.ReadDir("runtime")
	if err != nil {
		return err
	}
	for _, e := range entries {
		data, err := runtimeFS.ReadFile("runtime/" + e.Name())
		if err != nil {
			return err
		}
		out := strings.Replace(string(data), "package PACKAGE_NAME", "package "+pkg.Name, 1)
		out = strings.Replace(out, "BASE_URL_PLACEHOLDER", baseURL, 1)
		if err := os.WriteFile(filepath.Join(outDir, e.Name()), []byte(out), 0o644); err != nil {
			return err
		}
	}
	return nil
}
