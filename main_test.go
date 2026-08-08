package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGoldenSDK regenerates the SDK into a temp dir and diffs it against the
// committed output in sdk/pocketsmith. This makes generator changes reviewable
// as diffs of the generated code. If a change is intentional, re-run:
//
//	make generate
func TestGoldenSDK(t *testing.T) {
	tmp := t.TempDir()
	if err := run("openapi.json", tmp, "pocketsmith", ""); err != nil {
		t.Fatalf("generate: %v", err)
	}

	const golden = "sdk/pocketsmith"
	generated := map[string]bool{}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		generated[e.Name()] = true
		got, err := os.ReadFile(filepath.Join(tmp, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(golden, e.Name()))
		if err != nil {
			t.Errorf("%s: missing from committed output (run `make generate`): %v", e.Name(), err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%s: committed output is stale, run `make generate`", e.Name())
		}
	}

	// Committed files the generator no longer produces are stale too.
	goldenEntries, err := os.ReadDir(golden)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range goldenEntries {
		if !generated[e.Name()] {
			t.Errorf("%s: committed but no longer generated, run `make generate`", e.Name())
		}
	}
}
