package vault

import (
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

// ContractType enumerates the kinds of contracts supported.
// Phase 1 only implements http-api; others are reserved for future extension.
type ContractType string

const (
	ContractTypeHTTPAPI    ContractType = "http-api"
	ContractTypeMessage    ContractType = "message"
	ContractTypeUIProps    ContractType = "ui-props"
	ContractTypeDataSchema ContractType = "data-schema"
)

// ChangeType classifies a diff between two contract versions.
type ChangeType string

const (
	ChangeNone      ChangeType = "none"
	ChangeAdditive  ChangeType = "additive"
	ChangeBreaking  ChangeType = "breaking"
)

// ContractYAML is the on-disk shape of a contract file.
// Phase 1 only populates Meta, HTTP, Generates and Changelog for http-api type;
// other contract types reserve slots for future extension.
type ContractYAML struct {
	ID        string           `yaml:"id"`
	Version   string           `yaml:"version"`
	Status    string           `yaml:"status,omitempty"`
	Owners    []string         `yaml:"owners,omitempty"`
	Type      ContractType     `yaml:"type"`
	HTTP      *HTTPContract    `yaml:"http,omitempty"`
	Generates []ContractTarget `yaml:"generates,omitempty"`
	Changelog []ChangelogEntry `yaml:"changelog,omitempty"`
}

// HTTPContract captures the request/response shape for an http-api contract.
type HTTPContract struct {
	Method   string            `yaml:"method"`
	Path     string            `yaml:"path"`
	Request  *HTTPRequest      `yaml:"request,omitempty"`
	Response *HTTPResponse     `yaml:"response,omitempty"`
}

// HTTPRequest describes a request shape. Body/Query are intentionally loose
// (map[string]interface{}) so YAML can carry arbitrary structures.
type HTTPRequest struct {
	Query map[string]interface{} `yaml:"query,omitempty"`
	Body  map[string]interface{} `yaml:"body,omitempty"`
}

// HTTPResponse lists the success case and error cases.
type HTTPResponse struct {
	Success *HTTPSuccess `yaml:"success,omitempty"`
	Errors  []HTTPError  `yaml:"errors,omitempty"`
}

type HTTPSuccess struct {
	Status int                    `yaml:"status"`
	Body   map[string]interface{} `yaml:"body,omitempty"`
}

type HTTPError struct {
	Status int    `yaml:"status"`
	When   string `yaml:"when"`
}

// ContractTarget describes a generation target (project + format + output path).
type ContractTarget struct {
	Project string `yaml:"project"`
	Format  string `yaml:"format"`
	Path    string `yaml:"path"`
}

// ChangelogEntry records a contract version bump.
type ChangelogEntry struct {
	Version   string     `yaml:"version"`
	Date      string     `yaml:"date"`
	Type      ChangeType `yaml:"type"`
	Summary   string     `yaml:"summary"`
	Reason    string     `yaml:"reason,omitempty"`
	RetroItem string     `yaml:"retro_item,omitempty"`
}

// ParseContract unmarshals YAML bytes into a ContractYAML.
func ParseContract(data []byte) (*ContractYAML, error) {
	var c ContractYAML
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse contract: %w", err)
	}
	return &c, nil
}

// MarshalContract serializes a contract back to YAML bytes.
func MarshalContract(c *ContractYAML) ([]byte, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshal contract: %w", err)
	}
	return data, nil
}

// ClassifyChange compares an old and new version of the same contract and
// returns the change category. The diff algorithm is conservative: any uncertain
// change is classified as breaking so the human reviewer is forced to look.
//
// Rules (Phase 1, http-api only):
//   - If either side is nil → breaking (contract appearing/disappearing).
//   - Method or path changed → breaking.
//   - A field present in old response body is missing in new → breaking.
//   - A field's type changed between old and new response body → breaking.
//   - New field added only in new response body → additive.
//   - New error case added → additive.
//   - Nothing changed → none.
//
// Contracts of non-http-api types fall back to raw YAML string compare (any
// difference → breaking, same → none). Phase 2 can specialize further.
func ClassifyChange(oldC, newC *ContractYAML) ChangeType {
	if oldC == nil && newC == nil {
		return ChangeNone
	}
	if oldC == nil || newC == nil {
		return ChangeBreaking
	}

	if oldC.Type != newC.Type {
		return ChangeBreaking
	}

	if newC.Type != ContractTypeHTTPAPI {
		// Non-http-api types don't have a specialized comparator yet; compare the
		// stable shape fields (type + generates) and treat any difference as
		// breaking so the reviewer looks. Phase 2 can specialize per-type.
		if reflect.DeepEqual(oldC.Generates, newC.Generates) {
			return ChangeNone
		}
		return ChangeBreaking
	}

	if oldC.HTTP == nil || newC.HTTP == nil {
		return ChangeBreaking
	}
	if oldC.HTTP.Method != newC.HTTP.Method || oldC.HTTP.Path != newC.HTTP.Path {
		return ChangeBreaking
	}

	change := ChangeNone

	oldSuccessBody := map[string]interface{}{}
	newSuccessBody := map[string]interface{}{}
	if oldC.HTTP.Response != nil && oldC.HTTP.Response.Success != nil {
		oldSuccessBody = oldC.HTTP.Response.Success.Body
	}
	if newC.HTTP.Response != nil && newC.HTTP.Response.Success != nil {
		newSuccessBody = newC.HTTP.Response.Success.Body
	}

	// Fields in old but missing (or type-changed) in new → breaking.
	for k, oldV := range oldSuccessBody {
		newV, ok := newSuccessBody[k]
		if !ok {
			return ChangeBreaking
		}
		if typeTag(oldV) != typeTag(newV) {
			return ChangeBreaking
		}
	}
	// Fields only in new → additive.
	for k := range newSuccessBody {
		if _, ok := oldSuccessBody[k]; !ok {
			change = ChangeAdditive
		}
	}

	// Error cases: only count additive-or-same; removal is fine (usually tighter).
	if oldC.HTTP.Response != nil && newC.HTTP.Response != nil {
		if len(newC.HTTP.Response.Errors) > len(oldC.HTTP.Response.Errors) {
			change = ChangeAdditive
		}
	}

	return change
}

// typeTag returns a coarse type classifier used by ClassifyChange.
func typeTag(v interface{}) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case int, int32, int64, float32, float64:
		return "number"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}
