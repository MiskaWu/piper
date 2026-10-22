package tmux

import (
	"fmt"
	"strings"
	"testing"
)

func TestRequireTmux_已安裝時無錯誤(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return []byte("tmux 3.4"), nil
		},
	}
	client := NewClient(mock)

	err := client.RequireTmux()

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRequireTmux_未安裝時回傳錯誤含安裝提示(t *testing.T) {
	mock := &mockCommander{
		outputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("exec: \"tmux\": executable file not found in $PATH")
		},
	}
	client := NewClient(mock)

	err := client.RequireTmux()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	// 應包含安裝指引關鍵字
	for _, keyword := range []string{"tmux", "brew install", "apt install", "安裝方式"} {
		if !strings.Contains(msg, keyword) {
			t.Errorf("error message should contain %q, got: %s", keyword, msg)
		}
	}
}
