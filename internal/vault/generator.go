package vault

import (
	"bytes"
	_ "embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

//go:embed templates/go-httptest.go.tmpl
var goHTTPTestTmplStr string

//go:embed templates/ts-types.ts.tmpl
var tsTypesTmplStr string

var (
	goHTTPTestTmpl = template.Must(template.New("go-httptest").Parse(goHTTPTestTmplStr))
	tsTypesTmpl    = template.Must(template.New("ts-types").Parse(tsTypesTmplStr))
)

// typedField describes a field with its TypeScript type name.
type typedField struct {
	Name   string
	TSType string
}

// goTestView is the view model passed to the Go httptest template.
type goTestView struct {
	ContractID        string
	Version           string
	PackageName       string
	PascalID          string
	Method            string
	Path              string
	SuccessStatus     int
	SuccessBodyFields []string
	ErrorCases        []HTTPError
}

// tsTypesView is the view model passed to the TypeScript types template.
type tsTypesView struct {
	ContractID             string
	Version                string
	PascalID               string
	HasRequestQuery        bool
	HasRequestBody         bool
	RequestQueryFields     []typedField
	RequestBodyFields      []typedField
	SuccessBodyFieldsTyped []typedField
	ErrorStatuses          []int
}

// GenerateGoTest produces a go httptest skeleton for an http-api contract.
// The target file path (from generates[].path) decides the package name.
func GenerateGoTest(c *ContractYAML, targetPath string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("generate go test: contract is nil")
	}
	if c.Type != ContractTypeHTTPAPI {
		return "", fmt.Errorf("generate go test: type %q not supported (http-api only)", c.Type)
	}
	if c.HTTP == nil {
		return "", fmt.Errorf("generate go test: contract missing http block")
	}

	view := goTestView{
		ContractID:  c.ID,
		Version:     c.Version,
		PackageName: derivePackageName(targetPath),
		PascalID:    toPascal(c.ID),
		Method:      c.HTTP.Method,
		Path:        c.HTTP.Path,
	}

	if c.HTTP.Response != nil && c.HTTP.Response.Success != nil {
		view.SuccessStatus = c.HTTP.Response.Success.Status
		view.SuccessBodyFields = sortedKeys(c.HTTP.Response.Success.Body)
	}
	if view.SuccessStatus == 0 {
		view.SuccessStatus = 200
	}
	if c.HTTP.Response != nil {
		view.ErrorCases = c.HTTP.Response.Errors
	}

	var buf bytes.Buffer
	if err := goHTTPTestTmpl.Execute(&buf, view); err != nil {
		return "", fmt.Errorf("generate go test %s: %w", c.ID, err)
	}
	return buf.String(), nil
}

// GenerateTSTypes produces a TypeScript type declaration file for an http-api contract.
func GenerateTSTypes(c *ContractYAML) (string, error) {
	if c == nil {
		return "", fmt.Errorf("generate ts types: contract is nil")
	}
	if c.Type != ContractTypeHTTPAPI {
		return "", fmt.Errorf("generate ts types: type %q not supported (http-api only)", c.Type)
	}
	if c.HTTP == nil {
		return "", fmt.Errorf("generate ts types: contract missing http block")
	}

	view := tsTypesView{
		ContractID: c.ID,
		Version:    c.Version,
		PascalID:   toPascal(c.ID),
	}

	if c.HTTP.Request != nil {
		if len(c.HTTP.Request.Query) > 0 {
			view.HasRequestQuery = true
			view.RequestQueryFields = mapToTypedFields(c.HTTP.Request.Query)
		}
		if len(c.HTTP.Request.Body) > 0 {
			view.HasRequestBody = true
			view.RequestBodyFields = mapToTypedFields(c.HTTP.Request.Body)
		}
	}
	if c.HTTP.Response != nil {
		if c.HTTP.Response.Success != nil {
			view.SuccessBodyFieldsTyped = mapToTypedFields(c.HTTP.Response.Success.Body)
		}
		statusSeen := map[int]bool{}
		for _, e := range c.HTTP.Response.Errors {
			if !statusSeen[e.Status] {
				view.ErrorStatuses = append(view.ErrorStatuses, e.Status)
				statusSeen[e.Status] = true
			}
		}
		sort.Ints(view.ErrorStatuses)
	}

	var buf bytes.Buffer
	if err := tsTypesTmpl.Execute(&buf, view); err != nil {
		return "", fmt.Errorf("generate ts types %s: %w", c.ID, err)
	}
	return buf.String(), nil
}

// toPascal converts an id like "api-session-list" → "ApiSessionList".
func toPascal(id string) string {
	if id == "" {
		return "Contract"
	}
	parts := strings.FieldsFunc(id, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == '/' || unicode.IsSpace(r)
	})
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	if b.Len() == 0 {
		return "Contract"
	}
	return b.String()
}

// derivePackageName returns the package name from a target Go file path.
// e.g. "routes_sessions_contract_test.go" → "main", "internal/foo/bar_test.go" → "foo".
func derivePackageName(targetPath string) string {
	dir := filepath.Dir(targetPath)
	if dir == "" || dir == "." || dir == "/" {
		return "main"
	}
	base := filepath.Base(dir)
	if base == "" || base == "." || base == "/" {
		return "main"
	}
	// sanitize for package name (letters/digits/underscore)
	var b strings.Builder
	for i, r := range base {
		switch {
		case unicode.IsLetter(r):
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsDigit(r) && i > 0:
			b.WriteRune(r)
		case r == '_':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "main"
	}
	return b.String()
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys(m map[string]interface{}) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// mapToTypedFields converts a YAML map into typedField slice with TS types.
// The map value is interpreted as the TS type hint (string "string"/"number"/etc).
func mapToTypedFields(m map[string]interface{}) []typedField {
	if len(m) == 0 {
		return nil
	}
	keys := sortedKeys(m)
	fields := make([]typedField, 0, len(keys))
	for _, k := range keys {
		fields = append(fields, typedField{Name: k, TSType: toTSType(m[k])})
	}
	return fields
}

// toTSType maps a YAML value to a TypeScript type string.
// For string values, the string itself is treated as a type hint ("string", "number", "boolean", "array", "object", or a custom type).
func toTSType(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case int, int32, int64, float32, float64:
		return "number"
	case string:
		switch val {
		case "string", "number", "boolean", "null", "any", "unknown":
			return val
		case "array":
			return "unknown[]"
		case "object":
			return "Record<string, unknown>"
		default:
			return val
		}
	case []interface{}:
		return "unknown[]"
	case map[string]interface{}:
		return "Record<string, unknown>"
	default:
		return "unknown"
	}
}
