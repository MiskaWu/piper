package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	// ErrBindingNotFound is returned when a binding file does not exist.
	ErrBindingNotFound = errors.New("binding not found")
	// ErrInvalidContractID is returned when a contract id fails path-safety checks.
	ErrInvalidContractID = errors.New("invalid contract id")
	// ErrBindingInvalid is returned when a binding fails validation.
	ErrBindingInvalid = errors.New("invalid binding")
)

// Binding captures a project-local mapping from a contract to its concrete
// implementation artifacts (proto service.method, message names, etc.).
// Bindings live under {projectPath}/.plan/bindings/{contract-id}.yaml and are
// owned by the downstream project — contracts themselves remain upstream
// single-writer artifacts (governed by /design).
type Binding struct {
	ContractID      string        `yaml:"contract_id"`
	ContractVersion string        `yaml:"contract_version"`
	BoundAt         string        `yaml:"bound_at,omitempty"`
	Proto           *ProtoBinding `yaml:"proto,omitempty"`
	Notes           string        `yaml:"notes,omitempty"`
}

// ProtoBinding holds the proto-specific binding fields. All fields are
// required when the parent Binding has Proto set — partial proto bindings are
// rejected by ValidateBinding so /verify can trust the data.
type ProtoBinding struct {
	Service  string `yaml:"service"`
	Method   string `yaml:"method"`
	File     string `yaml:"file"`
	Request  string `yaml:"request"`
	Response string `yaml:"response"`
}

// BindingsDir returns the absolute path to {projectPath}/.plan/bindings.
func BindingsDir(projectPath string) string {
	return filepath.Join(projectPath, ".plan", "bindings")
}

// BindingPath returns the absolute path for a given contract id.
// contractID must not contain path separators or traversal components.
func BindingPath(projectPath, contractID string) (string, error) {
	cleaned := filepath.Clean(contractID)
	if cleaned != contractID ||
		strings.Contains(cleaned, "..") ||
		filepath.IsAbs(cleaned) ||
		strings.ContainsAny(cleaned, `/\`) {
		return "", fmt.Errorf("%w: %q", ErrInvalidContractID, contractID)
	}
	return filepath.Join(BindingsDir(projectPath), cleaned+".yaml"), nil
}

// ReadBinding loads a single binding file. Returns ErrBindingNotFound if the
// file is missing.
func ReadBinding(projectPath, contractID string) (*Binding, error) {
	path, err := BindingPath(projectPath, contractID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBindingNotFound
		}
		return nil, fmt.Errorf("read binding %s: %w", path, err)
	}
	b, err := ParseBinding(data)
	if err != nil {
		return nil, fmt.Errorf("parse binding %s: %w", path, err)
	}
	return b, nil
}

// WriteBinding serializes and writes a binding atomically (temp file + rename).
// Creates .plan/bindings/ if it does not exist. Validates before writing so
// malformed bindings never land on disk.
func WriteBinding(projectPath string, b *Binding) error {
	if err := ValidateBinding(b); err != nil {
		return fmt.Errorf("%w: %w", ErrBindingInvalid, err)
	}
	path, err := BindingPath(projectPath, b.ContractID)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create bindings dir: %w", err)
	}
	data, err := MarshalBinding(b)
	if err != nil {
		return err
	}
	// CreateTemp picks a unique suffix per call so concurrent PUTs on the same
	// contract id don't clobber each other's in-flight temp file.
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create binding tmp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write binding tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close binding tmp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename binding: %w", err)
	}
	return nil
}

// DeleteBinding removes a binding file. Returns ErrBindingNotFound if missing.
func DeleteBinding(projectPath, contractID string) error {
	path, err := BindingPath(projectPath, contractID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrBindingNotFound
		}
		return fmt.Errorf("delete binding %s: %w", path, err)
	}
	return nil
}

// ListBindings returns the sorted contract ids that have a binding in this
// project. Returns an empty slice if the bindings directory does not exist.
func ListBindings(projectPath string) ([]string, error) {
	dir := BindingsDir(projectPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(name, ".yaml"))
	}
	sort.Strings(ids)
	return ids, nil
}

// ParseBinding unmarshals YAML bytes into a Binding.
func ParseBinding(data []byte) (*Binding, error) {
	var b Binding
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("unmarshal binding: %w", err)
	}
	return &b, nil
}

// MarshalBinding serializes a Binding to YAML bytes.
func MarshalBinding(b *Binding) ([]byte, error) {
	data, err := yaml.Marshal(b)
	if err != nil {
		return nil, fmt.Errorf("marshal binding: %w", err)
	}
	return data, nil
}

// ValidateBinding checks that required fields are present. A binding must
// carry contract_id, contract_version, and at least one transport-specific
// binding block (currently only Proto). Proto blocks, when present, must have
// all fields filled — partial proto bindings make downstream test generators
// produce broken scaffolding, so we reject early.
func ValidateBinding(b *Binding) error {
	if b == nil {
		return errors.New("binding is nil")
	}
	var missing []string
	if strings.TrimSpace(b.ContractID) == "" {
		missing = append(missing, "contract_id is required")
	}
	if strings.TrimSpace(b.ContractVersion) == "" {
		missing = append(missing, "contract_version is required")
	}
	if b.Proto == nil {
		missing = append(missing, "at least one transport binding block is required (proto)")
	} else {
		for _, m := range validateProtoBinding(b.Proto) {
			missing = append(missing, "proto: "+m)
		}
	}
	if len(missing) > 0 {
		return errors.New(strings.Join(missing, "; "))
	}
	return nil
}

func validateProtoBinding(p *ProtoBinding) []string {
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"service", p.Service},
		{"method", p.Method},
		{"file", p.File},
		{"request", p.Request},
		{"response", p.Response},
	} {
		if strings.TrimSpace(f.value) == "" {
			missing = append(missing, f.name+" is required")
		}
	}
	return missing
}
