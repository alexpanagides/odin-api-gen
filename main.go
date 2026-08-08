// odin-api-gen generates an Odin SDK from an OpenAPI 3 spec.
//
// It is a build-time tool: it reads the spec and writes .odin files; nothing
// from this Go program ships at runtime. It deliberately supports only a
// bounded subset of OpenAPI (see README) and fails loudly on anything else.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"odin-api-gen/internal/gen"
)

func main() {
	spec := flag.String("spec", "openapi.json", "path to the OpenAPI 3 spec (JSON or YAML)")
	out := flag.String("out", "", "output directory for the generated Odin package (required)")
	pkg := flag.String("package", "", "Odin package name (required)")
	baseURL := flag.String("base-url", "", "override the API base URL (default: first server in the spec)")
	flag.Parse()

	if *out == "" || *pkg == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*spec, *out, *pkg, *baseURL); err != nil {
		fmt.Fprintln(os.Stderr, "odin-api-gen:", err)
		os.Exit(1)
	}
}

func run(specPath, outDir, pkgName, baseURL string) error {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		return fmt.Errorf("loading spec: %w", err)
	}
	// Examples don't affect codegen, and real-world specs (including
	// PocketSmith's) contain invalid ones.
	if err := doc.Validate(loader.Context, openapi3.DisableExamplesValidation()); err != nil {
		return fmt.Errorf("validating spec: %w", err)
	}

	if baseURL == "" {
		if len(doc.Servers) == 0 {
			return fmt.Errorf("spec has no servers; pass -base-url")
		}
		baseURL = strings.TrimSuffix(doc.Servers[0].URL, "/")
	}

	p, err := gen.Resolve(doc, pkgName)
	if err != nil {
		return err
	}
	if err := gen.Emit(p, filepath.Base(specPath), outDir, baseURL); err != nil {
		return err
	}

	nOps := 0
	for _, g := range p.Groups {
		nOps += len(g.Ops)
	}
	fmt.Printf("generated %d models, %d operations into %s\n", len(p.Models), nOps, outDir)
	return nil
}
