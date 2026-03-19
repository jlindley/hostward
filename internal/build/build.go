package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Build(outputPath string) error {
	root, err := FindModuleRoot()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output dir for %s: %w", outputPath, err)
	}

	cmd := exec.Command("go", "build", "-o", outputPath, "./cmd/hostward")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func FindModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working dir: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod from current directory")
		}
		dir = parent
	}
}
