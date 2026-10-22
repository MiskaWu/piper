package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleHTTPContract() *ContractYAML {
	return &ContractYAML{
		ID:      "api-session-list",
		Version: "1.0.0",
		Type:    ContractTypeHTTPAPI,
		HTTP: &HTTPContract{
			Method: "GET",
			Path:   "/api/sessions",
			Request: &HTTPRequest{
				Query: map[string]interface{}{
					"limit": "number",
					"tag":   "string",
				},
			},
			Response: &HTTPResponse{
				Success: &HTTPSuccess{
					Status: 200,
					Body: map[string]interface{}{
						"items": "array",
						"total": "number",
					},
				},
				Errors: []HTTPError{
					{Status: 400, When: "invalid query"},
					{Status: 500, When: "internal error"},
				},
			},
		},
	}
}

func TestGenerateGoTest(t *testing.T) {
	c := sampleHTTPContract()
	out, err := GenerateGoTest(c, "routes_sessions_contract_test.go")
	if err != nil {
		t.Fatalf("GenerateGoTest: %v", err)
	}

	checks := []string{
		"package main",
		"func TestApiSessionListContract",
		`"GET"`,
		`"/api/sessions"`,
		`"items"`,
		`"total"`,
		"wantStatus: 200",
		"wantStatus: 400",
		"wantStatus: 500",
		`"invalid query"`,
		`"internal error"`,
	}
	for _, s := range checks {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q\n---\n%s", s, out)
		}
	}
}

func TestGenerateGoTest_PackageNameFromDir(t *testing.T) {
	c := sampleHTTPContract()
	out, err := GenerateGoTest(c, "internal/sessions/contract_test.go")
	if err != nil {
		t.Fatalf("GenerateGoTest: %v", err)
	}
	if !strings.Contains(out, "package sessions") {
		t.Errorf("expected 'package sessions' for internal/sessions/ path, got:\n%s", out)
	}
}

func TestGenerateGoTest_RejectsNonHTTP(t *testing.T) {
	c := &ContractYAML{ID: "msg", Type: ContractTypeMessage}
	if _, err := GenerateGoTest(c, "x_test.go"); err == nil {
		t.Error("expected error for non-http-api type")
	}
}

func TestGenerateTSTypes(t *testing.T) {
	c := sampleHTTPContract()
	out, err := GenerateTSTypes(c)
	if err != nil {
		t.Fatalf("GenerateTSTypes: %v", err)
	}

	checks := []string{
		"export interface ApiSessionListRequestQuery",
		"limit: number",
		"tag: string",
		"export interface ApiSessionListResponse",
		"items: unknown[]",
		"total: number",
		"export type ApiSessionListErrorStatus = 400 | 500",
	}
	for _, s := range checks {
		if !strings.Contains(out, s) {
			t.Errorf("ts output missing %q\n---\n%s", s, out)
		}
	}
}

func TestGenerateTSTypes_NoRequest(t *testing.T) {
	c := &ContractYAML{
		ID:      "api-healthz",
		Version: "1",
		Type:    ContractTypeHTTPAPI,
		HTTP: &HTTPContract{
			Method: "GET", Path: "/healthz",
			Response: &HTTPResponse{
				Success: &HTTPSuccess{Status: 200, Body: map[string]interface{}{"ok": "boolean"}},
			},
		},
	}
	out, err := GenerateTSTypes(c)
	if err != nil {
		t.Fatalf("GenerateTSTypes: %v", err)
	}
	if strings.Contains(out, "RequestQuery") || strings.Contains(out, "RequestBody") {
		t.Errorf("should not emit Request interface when request is empty:\n%s", out)
	}
	if !strings.Contains(out, "ok: boolean") {
		t.Errorf("missing response field: %s", out)
	}
	if !strings.Contains(out, "ErrorStatus = never") {
		t.Errorf("no errors should produce 'never':\n%s", out)
	}
}

func TestToPascal(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"api-session-list", "ApiSessionList"},
		{"api_session_list", "ApiSessionList"},
		{"simple", "Simple"},
		{"", "Contract"},
		{"----", "Contract"},
	}
	for _, tt := range tests {
		if got := toPascal(tt.in); got != tt.want {
			t.Errorf("toPascal(%q): got %q, want %q", tt.in, got, tt.want)
		}
	}
}

// setupContractForGenerate creates a vault, epic, and contract with a single
// generates[] target, returning everything GenerateContract needs.
func setupContractForGenerate(t *testing.T, targetPath string) (*VaultManager, string, string, string) {
	t.Helper()
	v, _ := newTestVault(t)
	epicID, err := v.CreateEpic("Gen Test")
	if err != nil {
		t.Fatalf("CreateEpic: %v", err)
	}
	contractYAML := `id: api-session-list
version: "1.0"
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
        when: internal
generates:
  - project: myproj
    format: go-httptest
    path: ` + targetPath + "\n"
	if err := v.WriteContract(epicID, "api-session-list", contractYAML); err != nil {
		t.Fatalf("WriteContract: %v", err)
	}
	projectRoot := t.TempDir()
	return v, epicID, "api-session-list", projectRoot
}

func TestGenerateContract_StaleWarning(t *testing.T) {
	t.Run("target 父目錄不存在時產出 warning", func(t *testing.T) {
		v, epicID, contractID, projectRoot := setupContractForGenerate(t, "does/not/exist/routes_sessions_contract_test.go")
		resolve := func(name string) (string, bool) { return projectRoot, true }

		artifacts, err := v.GenerateContract(epicID, contractID, resolve, GenerateOptions{DryRun: true})
		if err != nil {
			t.Fatalf("GenerateContract: %v", err)
		}
		if len(artifacts) != 1 {
			t.Fatalf("expected 1 artifact, got %d", len(artifacts))
		}
		if len(artifacts[0].Warnings) == 0 {
			t.Fatalf("expected warning for stale path, got none")
		}
		if artifacts[0].Warnings[0].Code != WarnStaleManifest {
			t.Errorf("warning code = %q, want %q", artifacts[0].Warnings[0].Code, WarnStaleManifest)
		}
	})

	t.Run("target 父目錄存在時沒有 warning", func(t *testing.T) {
		v, epicID, contractID, projectRoot := setupContractForGenerate(t, "sub/routes_sessions_contract_test.go")
		// Pre-create the parent dir so stat succeeds.
		if err := os.MkdirAll(filepath.Join(projectRoot, "sub"), 0o755); err != nil {
			t.Fatalf("setup parent dir: %v", err)
		}
		resolve := func(name string) (string, bool) { return projectRoot, true }

		artifacts, err := v.GenerateContract(epicID, contractID, resolve, GenerateOptions{DryRun: true})
		if err != nil {
			t.Fatalf("GenerateContract: %v", err)
		}
		if len(artifacts) != 1 {
			t.Fatalf("expected 1 artifact, got %d", len(artifacts))
		}
		if len(artifacts[0].Warnings) != 0 {
			t.Errorf("expected no warning, got %v", artifacts[0].Warnings)
		}
	})

	t.Run("resolve 回傳 ok=false 時不產 warning（沒 project path 無從檢查）", func(t *testing.T) {
		v, epicID, contractID, _ := setupContractForGenerate(t, "does/not/exist/x_test.go")
		resolve := func(name string) (string, bool) { return "", false }

		artifacts, err := v.GenerateContract(epicID, contractID, resolve, GenerateOptions{DryRun: true})
		if err != nil {
			t.Fatalf("GenerateContract: %v", err)
		}
		if len(artifacts[0].Warnings) != 0 {
			t.Errorf("expected no warning when resolver returns not-ok, got %v", artifacts[0].Warnings)
		}
	})
}
