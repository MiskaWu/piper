package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrManifestNotFound is returned when .plan/manifest.yaml is missing.
var ErrManifestNotFound = errors.New("manifest not found")

// ManifestPath returns the absolute path to .plan/manifest.yaml under projectPath.
func ManifestPath(projectPath string) string {
	return filepath.Join(projectPath, ".plan", "manifest.yaml")
}

// ReadManifest reads the raw YAML bytes of .plan/manifest.yaml.
// Returns ErrManifestNotFound if the file does not exist.
func ReadManifest(projectPath string) ([]byte, error) {
	path := ManifestPath(projectPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrManifestNotFound
		}
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return data, nil
}
