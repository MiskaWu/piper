package devcontainer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectVersions_UpdateAvailable_版本一致無需更新(t *testing.T) {
	v := ProjectVersions{
		TemplateVersion: ConfigTemplateVersion,
	}
	if v.UpdateAvailable() {
		t.Error("expected no update available when versions match")
	}
}

func TestProjectVersions_UpdateAvailable_template版本落後需更新(t *testing.T) {
	v := ProjectVersions{
		TemplateVersion: 0,
	}
	if !v.UpdateAvailable() {
		t.Error("expected update available when template version is behind")
	}
}

func TestGetProjectVersions_有版本metadata(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)
	os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(`{
		"name": "test",
		"piper": { "templateVersion": 5 }
	}`), 0o644)

	v := GetProjectVersions(dir)
	if v.TemplateVersion != 5 {
		t.Errorf("expected templateVersion=5, got %d", v.TemplateVersion)
	}
}

func TestGetProjectVersions_無piper欄位回傳零值(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)
	os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(`{
		"name": "test"
	}`), 0o644)

	v := GetProjectVersions(dir)
	if v.TemplateVersion != 0 {
		t.Errorf("expected zero version, got template=%d", v.TemplateVersion)
	}
}

func TestGetProjectVersions_檔案不存在回傳零值(t *testing.T) {
	v := GetProjectVersions("/nonexistent/path")
	if v.TemplateVersion != 0 {
		t.Errorf("expected zero version for nonexistent path")
	}
}

func TestExtractLanguageFragment_提取語言片段_新格式(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)

	dockerfile := "FROM node:22-bookworm-slim\nRUN apt-get update\n\n" + languageLayerMarker + "\n\nRUN curl -fsSL https://go.dev/dl/go1.26.1.linux-amd64.tar.gz | tar -C /usr/local -xzf -\nENV GOROOT=/usr/local/go\n"
	os.WriteFile(filepath.Join(dcDir, "Dockerfile"), []byte(dockerfile), 0o644)

	fragment, err := ExtractLanguageFragment(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(fragment, "go1.26.1") {
		t.Errorf("expected fragment to contain go install, got: %s", fragment)
	}
}

func TestExtractLanguageFragment_提取語言片段_舊格式fallback(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)

	dockerfile := "FROM piper-devcontainer-base\n\nRUN curl -fsSL https://go.dev/dl/go1.26.1.linux-amd64.tar.gz | tar -C /usr/local -xzf -\nENV GOROOT=/usr/local/go\n"
	os.WriteFile(filepath.Join(dcDir, "Dockerfile"), []byte(dockerfile), 0o644)

	fragment, err := ExtractLanguageFragment(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(fragment, "go1.26.1") {
		t.Errorf("expected fragment to contain go install, got: %s", fragment)
	}
}

func TestExtractLanguageFragment_檔案不存在回傳錯誤(t *testing.T) {
	_, err := ExtractLanguageFragment("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestExtractVSCodeExtensions_過濾claude_code擴展(t *testing.T) {
	dir := t.TempDir()
	dcDir := filepath.Join(dir, ".devcontainer")
	os.MkdirAll(dcDir, 0o755)
	os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(`{
		"customizations": {
			"vscode": {
				"extensions": ["anthropic.claude-code", "golang.go", "ms-python.python"]
			}
		}
	}`), 0o644)

	exts := ExtractVSCodeExtensions(dir)
	if len(exts) != 2 {
		t.Fatalf("expected 2 extensions, got %d: %v", len(exts), exts)
	}
	if exts[0] != "golang.go" || exts[1] != "ms-python.python" {
		t.Errorf("unexpected extensions: %v", exts)
	}
}

func TestExtractVSCodeExtensions_檔案不存在回傳nil(t *testing.T) {
	exts := ExtractVSCodeExtensions("/nonexistent/path")
	if exts != nil {
		t.Errorf("expected nil, got %v", exts)
	}
}

