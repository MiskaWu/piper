package digest

import (
	"fmt"
	"strings"
	"testing"
)

// fakeCommander records calls and returns preset results.
type fakeCommander struct {
	output []byte
	err    error
	calls  []fakeCall
}

type fakeCall struct {
	name string
	args []string
}

func (f *fakeCommander) Output(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{name: name, args: args})
	return f.output, f.err
}

func TestSummarizeProject_正常歸納(t *testing.T) {
	fake := &fakeCommander{
		output: []byte("完成了登入功能的開發，並修復了相關的 CSS 樣式問題。"),
	}
	s := NewSummarizer(fake, "sonnet")

	result := s.SummarizeProject("piper", []string{"feat: 新增登入功能", "fix: 修復 CSS"}, []string{"查證 Keycloak OAuth 設定"})

	if result != "完成了登入功能的開發，並修復了相關的 CSS 樣式問題。" {
		t.Errorf("unexpected result: %q", result)
	}

	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}

	call := fake.calls[0]
	if call.name != "claude" {
		t.Errorf("expected command 'claude', got %q", call.name)
	}

	// Verify args contain correct model
	foundModel := false
	for i, arg := range call.args {
		if arg == "--model" && i+1 < len(call.args) && call.args[i+1] == "sonnet" {
			foundModel = true
		}
	}
	if !foundModel {
		t.Errorf("expected --model sonnet in args: %v", call.args)
	}

	// Verify the prompt (last arg) contains commits and conversations
	prompt := call.args[len(call.args)-1]
	if !strings.Contains(prompt, "piper") {
		t.Errorf("prompt should contain project name, got: %s", prompt)
	}
	if !strings.Contains(prompt, "feat: 新增登入功能") {
		t.Errorf("prompt should contain commit message")
	}
	if !strings.Contains(prompt, "查證 Keycloak OAuth 設定") {
		t.Errorf("prompt should contain conversation")
	}
}

func TestSummarizeProject_空輸入直接回傳(t *testing.T) {
	fake := &fakeCommander{}
	s := NewSummarizer(fake, "sonnet")

	result := s.SummarizeProject("piper", nil, nil)

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
	if len(fake.calls) != 0 {
		t.Errorf("expected no calls, got %d", len(fake.calls))
	}
}

func TestSummarizeProject_空slice也不呼叫AI(t *testing.T) {
	fake := &fakeCommander{}
	s := NewSummarizer(fake, "sonnet")

	result := s.SummarizeProject("piper", []string{}, []string{})

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
	if len(fake.calls) != 0 {
		t.Errorf("expected no calls, got %d", len(fake.calls))
	}
}

func TestSummarizeProject_AI呼叫失敗回傳空字串(t *testing.T) {
	fake := &fakeCommander{
		err: fmt.Errorf("claude process failed"),
	}
	s := NewSummarizer(fake, "sonnet")

	result := s.SummarizeProject("piper", []string{"feat: something"}, nil)

	if result != "" {
		t.Errorf("expected empty string on error, got %q", result)
	}
}

func TestSummarizeProject_只有commits(t *testing.T) {
	fake := &fakeCommander{
		output: []byte("新增了登入功能。"),
	}
	s := NewSummarizer(fake, "haiku")

	result := s.SummarizeProject("myapp", []string{"feat: 新增登入功能"}, nil)

	if result != "新增了登入功能。" {
		t.Errorf("unexpected result: %q", result)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}

	// Verify model
	call := fake.calls[0]
	foundModel := false
	for i, arg := range call.args {
		if arg == "--model" && i+1 < len(call.args) && call.args[i+1] == "haiku" {
			foundModel = true
		}
	}
	if !foundModel {
		t.Errorf("expected --model haiku in args: %v", call.args)
	}

	// Prompt should have empty conversations section
	prompt := call.args[len(call.args)-1]
	if !strings.Contains(prompt, "[Git Commits]") {
		t.Errorf("prompt should contain [Git Commits] section")
	}
}

func TestSummarizeProject_只有conversations(t *testing.T) {
	fake := &fakeCommander{
		output: []byte("研究了 OAuth 設定方向。"),
	}
	s := NewSummarizer(fake, "")

	result := s.SummarizeProject("myapp", nil, []string{"查證 Keycloak OAuth 設定"})

	if result != "研究了 OAuth 設定方向。" {
		t.Errorf("unexpected result: %q", result)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}

	// Empty model should default to "sonnet"
	call := fake.calls[0]
	foundModel := false
	for i, arg := range call.args {
		if arg == "--model" && i+1 < len(call.args) && call.args[i+1] == "sonnet" {
			foundModel = true
		}
	}
	if !foundModel {
		t.Errorf("expected --model sonnet (default) in args: %v", call.args)
	}
}
