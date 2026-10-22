package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseContract_RoundTrip(t *testing.T) {
	yaml := `id: api-sessions-list
version: "1.0.0"
type: http-api
http:
  method: GET
  path: /api/sessions
  response:
    success:
      status: 200
      body:
        items: array
    errors:
      - status: 500
        when: internal error
generates:
  - project: piper
    format: go-httptest
    path: routes_sessions_contract_test.go
changelog:
  - version: "1.0.0"
    date: "2026-04-17"
    type: additive
    summary: initial
`
	c, err := ParseContract([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.ID != "api-sessions-list" {
		t.Errorf("ID: got %q", c.ID)
	}
	if c.Type != ContractTypeHTTPAPI {
		t.Errorf("Type: got %q", c.Type)
	}
	if c.HTTP == nil || c.HTTP.Method != "GET" || c.HTTP.Path != "/api/sessions" {
		t.Errorf("HTTP: got %+v", c.HTTP)
	}
	if len(c.Generates) != 1 || c.Generates[0].Format != "go-httptest" {
		t.Errorf("Generates: got %+v", c.Generates)
	}
	if len(c.Changelog) != 1 || c.Changelog[0].Type != ChangeAdditive {
		t.Errorf("Changelog: got %+v", c.Changelog)
	}

	out, err := MarshalContract(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	c2, err := ParseContract(out)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if c2.ID != c.ID || c2.HTTP.Path != c.HTTP.Path {
		t.Errorf("round-trip mismatch")
	}
}

func TestClassifyChange(t *testing.T) {
	mkHTTP := func(method, path string, successBody map[string]interface{}, errors []HTTPError) *ContractYAML {
		return &ContractYAML{
			ID:      "test",
			Version: "1",
			Type:    ContractTypeHTTPAPI,
			HTTP: &HTTPContract{
				Method: method,
				Path:   path,
				Response: &HTTPResponse{
					Success: &HTTPSuccess{Status: 200, Body: successBody},
					Errors:  errors,
				},
			},
		}
	}

	tests := []struct {
		name string
		old  *ContractYAML
		new  *ContractYAML
		want ChangeType
	}{
		{
			name: "identical → none",
			old:  mkHTTP("GET", "/a", map[string]interface{}{"x": "string"}, nil),
			new:  mkHTTP("GET", "/a", map[string]interface{}{"x": "string"}, nil),
			want: ChangeNone,
		},
		{
			name: "method changed → breaking",
			old:  mkHTTP("GET", "/a", nil, nil),
			new:  mkHTTP("POST", "/a", nil, nil),
			want: ChangeBreaking,
		},
		{
			name: "path changed → breaking",
			old:  mkHTTP("GET", "/a", nil, nil),
			new:  mkHTTP("GET", "/b", nil, nil),
			want: ChangeBreaking,
		},
		{
			name: "field removed → breaking",
			old:  mkHTTP("GET", "/a", map[string]interface{}{"x": "string", "y": "number"}, nil),
			new:  mkHTTP("GET", "/a", map[string]interface{}{"x": "string"}, nil),
			want: ChangeBreaking,
		},
		{
			name: "field type changed → breaking",
			old:  mkHTTP("GET", "/a", map[string]interface{}{"x": "string"}, nil),
			new:  mkHTTP("GET", "/a", map[string]interface{}{"x": 123}, nil),
			want: ChangeBreaking,
		},
		{
			name: "new optional field → additive",
			old:  mkHTTP("GET", "/a", map[string]interface{}{"x": "s"}, nil),
			new:  mkHTTP("GET", "/a", map[string]interface{}{"x": "s", "y": "s"}, nil),
			want: ChangeAdditive,
		},
		{
			name: "new error case → additive",
			old:  mkHTTP("GET", "/a", nil, []HTTPError{{Status: 400, When: "bad"}}),
			new:  mkHTTP("GET", "/a", nil, []HTTPError{{Status: 400, When: "bad"}, {Status: 500, When: "err"}}),
			want: ChangeAdditive,
		},
		{
			name: "type category changed → breaking",
			old:  &ContractYAML{ID: "x", Type: ContractTypeHTTPAPI, HTTP: &HTTPContract{Method: "GET", Path: "/a"}},
			new:  &ContractYAML{ID: "x", Type: ContractTypeMessage},
			want: ChangeBreaking,
		},
		{
			name: "old nil → breaking",
			old:  nil,
			new:  mkHTTP("GET", "/a", nil, nil),
			want: ChangeBreaking,
		},
		{
			name: "both nil → none",
			old:  nil,
			new:  nil,
			want: ChangeNone,
		},
		{
			name: "http nil on one side → breaking",
			old:  &ContractYAML{ID: "x", Type: ContractTypeHTTPAPI, HTTP: &HTTPContract{Method: "GET", Path: "/a"}},
			new:  &ContractYAML{ID: "x", Type: ContractTypeHTTPAPI},
			want: ChangeBreaking,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyChange(tt.old, tt.new)
			if got != tt.want {
				t.Errorf("ClassifyChange: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListContractsByOwner(t *testing.T) {
	v, _ := newTestVault(t)
	epicID, err := v.CreateEpic("Owner Test")
	if err != nil {
		t.Fatalf("CreateEpic: %v", err)
	}

	mkContract := func(id string, owners []string) string {
		ownerLines := ""
		if len(owners) > 0 {
			ownerLines = "owners:\n"
			for _, o := range owners {
				ownerLines += "  - " + o + "\n"
			}
		}
		return "id: " + id + "\n" +
			`version: "1.0"` + "\n" +
			"type: http-api\n" +
			ownerLines +
			"http:\n" +
			"  method: GET\n" +
			"  path: /api/" + id + "\n"
	}

	contracts := map[string][]string{
		"c-alpha": {"task-a1"},
		"c-beta":  {"task-a1", "task-a2"},
		"c-gamma": {"task-a2"},
		"c-delta": nil,
	}
	for id, owners := range contracts {
		if err := v.WriteContract(epicID, id, mkContract(id, owners)); err != nil {
			t.Fatalf("WriteContract %s: %v", id, err)
		}
	}

	// Stale / unparseable file should be skipped, not crash.
	contractsDir := filepath.Join(v.baseDir, "epics", epicID, "contracts")
	if err := os.WriteFile(filepath.Join(contractsDir, "c-broken.yaml"), []byte("id: [unclosed"), 0o644); err != nil {
		t.Fatalf("write broken contract: %v", err)
	}

	tests := []struct {
		name    string
		owner   string
		want    []string
		wantErr string
	}{
		{
			name:  "單一 owner 命中一份契約",
			owner: "task-a1",
			want:  []string{"c-alpha", "c-beta"},
		},
		{
			name:  "不同 owner 只拿到自己的契約",
			owner: "task-a2",
			want:  []string{"c-beta", "c-gamma"},
		},
		{
			name:  "無任何契約 own 的 task 回空清單",
			owner: "task-nonexistent",
			want:  nil,
		},
		{
			name:    "空 owner 參數回錯",
			owner:   "",
			wantErr: "owner is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v.ListContractsByOwner(epicID, tt.owner)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestValidateContract(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "valid http-api",
			yaml: `id: ok
version: "1"
type: http-api
http:
  method: GET
  path: /x
`,
		},
		{
			name: "missing id",
			yaml: `version: "1"
type: http-api
http:
  method: GET
  path: /x
`,
			wantErr: "id",
		},
		{
			name: "missing version",
			yaml: `id: x
type: http-api
http:
  method: GET
  path: /x
`,
			wantErr: "version",
		},
		{
			name: "missing type",
			yaml: `id: x
version: "1"
`,
			wantErr: "type",
		},
		{
			name: "unknown type",
			yaml: `id: x
version: "1"
type: quantum-flux
`,
			wantErr: "unknown type",
		},
		{
			name: "http-api missing http",
			yaml: `id: x
version: "1"
type: http-api
`,
			wantErr: "http",
		},
		{
			name: "http-api missing method",
			yaml: `id: x
version: "1"
type: http-api
http:
  path: /x
`,
			wantErr: "method",
		},
		{
			name: "http-api missing path",
			yaml: `id: x
version: "1"
type: http-api
http:
  method: GET
`,
			wantErr: "path",
		},
		{
			name:    "invalid yaml",
			yaml:    "id: [unclosed\n  nesting: broken",
			wantErr: "parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContract(tt.yaml)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
