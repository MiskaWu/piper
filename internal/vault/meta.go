package vault

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type metaData struct {
	NextIdeaID int `yaml:"nextIdeaId"`
}

func loadMeta(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("load meta %s: %w", path, err)
	}
	var m metaData
	if err := yaml.Unmarshal(data, &m); err != nil {
		return 0, fmt.Errorf("parse meta %s: %w", path, err)
	}
	return m.NextIdeaID, nil
}

func saveMeta(path string, nextID int) error {
	m := metaData{NextIdeaID: nextID}
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write meta %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename meta %s: %w", path, err)
	}
	return nil
}
