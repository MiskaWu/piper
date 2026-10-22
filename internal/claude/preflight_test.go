package claude

import (
	"fmt"
	"strings"
	"testing"
)

type mockCommander struct {
	outputFunc func(name string, args ...string) ([]byte, error)
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	return m.outputFunc(name, args...)
}

func TestRequireClaude_已安裝時無錯誤(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("1.0.16 (Claude Code)"), nil
		},
	}

	err := RequireClaude(mock)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRequireClaude_未安裝時回傳錯誤含安裝提示(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("exec: \"claude\": executable file not found in $PATH")
		},
	}

	err := RequireClaude(mock)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	for _, keyword := range []string{"claude", "npm install", "安裝方式"} {
		if !strings.Contains(msg, keyword) {
			t.Errorf("error message should contain %q, got: %s", keyword, msg)
		}
	}
}
