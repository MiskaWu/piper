package browse

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveConflict returns a non-conflicting name in targetDir.
// For "file.txt" it tries "file (1).txt", "file (2).txt", etc.
// For directories or extensionless files: "name (1)", "name (2)", etc.
func resolveConflict(targetDir, name string) string {
	if _, err := os.Stat(filepath.Join(targetDir, name)); os.IsNotExist(err) {
		return name
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(filepath.Join(targetDir, candidate)); os.IsNotExist(err) {
			return candidate
		}
	}
}
