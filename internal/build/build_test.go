package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindModuleRootFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() { _ = os.Chdir(previous) }()

	if err := os.Chdir(nested); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	got, err := FindModuleRoot()
	if err != nil {
		t.Fatalf("FindModuleRoot() error = %v", err)
	}
	gotEval, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(got) error = %v", err)
	}
	rootEval, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(root) error = %v", err)
	}
	if gotEval != rootEval {
		t.Fatalf("FindModuleRoot() = %q, want %q", gotEval, rootEval)
	}
}
