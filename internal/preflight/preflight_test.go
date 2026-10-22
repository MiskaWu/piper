package preflight

import (
	"fmt"
	"strings"
	"testing"
)

type mockCommander struct {
	available map[string]bool
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	if m.available[name] {
		return []byte("ok"), nil
	}
	return nil, fmt.Errorf("exec: %q: executable file not found in $PATH", name)
}

func TestRunAll_全部安裝時無錯誤(t *testing.T) {
	cmd := &mockCommander{available: map[string]bool{
		"tmux": true, "claude": true, "git": true, "docker": true, "bwrap": true, "socat": true,
	}}
	deps := []Dependency{
		{Name: "tmux", Cmd: "tmux", Args: []string{"-V"}, Required: true},
		{Name: "git", Cmd: "git", Args: []string{"--version"}, Required: false},
	}

	results := RunAll(cmd, deps)

	for _, r := range results {
		if !r.OK {
			t.Errorf("expected %s to be OK", r.Dep.Name)
		}
	}
}

func TestRunAll_必要依賴缺少時標記失敗(t *testing.T) {
	cmd := &mockCommander{available: map[string]bool{}}
	deps := []Dependency{
		{Name: "tmux", Cmd: "tmux", Args: []string{"-V"}, Required: true, InstallHint: "brew install tmux"},
	}

	results := RunAll(cmd, deps)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].OK {
		t.Fatal("expected tmux to fail")
	}
}

func TestRunAll_可選依賴缺少時標記失敗但不影響必要項(t *testing.T) {
	cmd := &mockCommander{available: map[string]bool{"tmux": true}}
	deps := []Dependency{
		{Name: "tmux", Cmd: "tmux", Args: []string{"-V"}, Required: true},
		{Name: "docker", Cmd: "docker", Args: []string{"--version"}, Required: false, Feature: "DevContainer"},
	}

	results := RunAll(cmd, deps)

	if !results[0].OK {
		t.Error("tmux should be OK")
	}
	if results[1].OK {
		t.Error("docker should fail")
	}
	if results[1].Dep.Required {
		t.Error("docker should be optional")
	}
}

func TestHasRequiredFailures_有必要依賴失敗(t *testing.T) {
	results := []Result{
		{Dep: Dependency{Name: "tmux", Required: true}, OK: false},
		{Dep: Dependency{Name: "docker", Required: false}, OK: false},
	}

	if !HasRequiredFailures(results) {
		t.Error("should detect required failure")
	}
}

func TestHasRequiredFailures_只有可選失敗(t *testing.T) {
	results := []Result{
		{Dep: Dependency{Name: "tmux", Required: true}, OK: true},
		{Dep: Dependency{Name: "docker", Required: false}, OK: false},
	}

	if HasRequiredFailures(results) {
		t.Error("should not report required failure when only optional fails")
	}
}

func TestFormatResults_格式包含安裝提示(t *testing.T) {
	results := []Result{
		{Dep: Dependency{Name: "tmux", Required: true}, OK: true},
		{Dep: Dependency{Name: "docker", Required: false, Feature: "DevContainer", InstallHint: "sudo apt install docker.io"}, OK: false},
	}

	output := FormatResults(results)

	if !strings.Contains(output, "tmux") {
		t.Error("output should contain tmux")
	}
	if !strings.Contains(output, "docker") {
		t.Error("output should contain docker")
	}
	if !strings.Contains(output, "DevContainer") {
		t.Error("output should mention affected feature")
	}
	if !strings.Contains(output, "sudo apt install docker.io") {
		t.Error("output should contain install hint")
	}
}

func TestFormatResults_必要失敗顯示必要標記(t *testing.T) {
	results := []Result{
		{Dep: Dependency{Name: "tmux", Required: true, InstallHint: "brew install tmux"}, OK: false},
	}

	output := FormatResults(results)

	if !strings.Contains(output, "✗") {
		t.Error("output should contain failure marker")
	}
}

func TestFindResult_找到指定dependency(t *testing.T) {
	results := []Result{
		{Dep: Dependency{Name: "tmux"}, OK: true},
		{Dep: Dependency{Name: "docker"}, OK: false},
	}
	r, found := FindResult(results, "docker")
	if !found {
		t.Fatal("expected to find docker")
	}
	if r.OK {
		t.Error("expected docker to be not OK")
	}
}

func TestFindResult_找不到時回傳false(t *testing.T) {
	results := []Result{
		{Dep: Dependency{Name: "tmux"}, OK: true},
	}
	_, found := FindResult(results, "missing")
	if found {
		t.Error("expected not found")
	}
}

func TestFindResult_空slice(t *testing.T) {
	_, found := FindResult(nil, "docker")
	if found {
		t.Error("expected not found for nil slice")
	}
}

func TestDefaultDeps_包含核心依賴(t *testing.T) {
	deps := DefaultDeps()

	names := make(map[string]bool)
	for _, d := range deps {
		names[d.Name] = true
	}

	for _, required := range []string{"tmux", "claude"} {
		if !names[required] {
			t.Errorf("default deps should include %s", required)
		}
	}
}
