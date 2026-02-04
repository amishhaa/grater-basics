package internal

import (
	"os"
	"path/filepath"
)

func EnsureWorkspace(dir string) (string, error) {
	if dir == "" {
		dir = ".grater"
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	return filepath.Abs(dir)
}

func WSPath(ws, name string) string {
	return filepath.Join(ws, name)
}
