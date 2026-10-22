package plan

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBindingPath(t *testing.T) {
	tests := []struct {
		name       string
		contractID string
		wantErr    bool
	}{
		{"普通 id 解析出 .plan/bindings/{id}.yaml", "dungeon-session-list", false},
		{"含斜線時被拒", "bad/id", true},
		{"含反斜線時被拒", `bad\id`, true},
		{"含 path traversal 時被拒", "..", true},
		{"絕對路徑時被拒", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BindingPath("/tmp/proj", tt.contractID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got path %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := filepath.Join("/tmp/proj", ".plan", "bindings", tt.contractID+".yaml")
			if got != want {
				t.Fatalf("BindingPath = %q, want %q", got, want)
			}
		})
	}
}

func TestValidateBinding(t *testing.T) {
	valid := &Binding{
		ContractID:      "dungeon-session-list",
		ContractVersion: "1.0",
		Proto: &ProtoBinding{
			Service:  "SessionService",
			Method:   "List",
			File:     "proto/sessions.proto",
			Request:  "ListSessionsRequest",
			Response: "ListSessionsResponse",
		},
	}

	tests := []struct {
		name       string
		mutate     func(b *Binding)
		wantErrMsg string
	}{
		{"完整 binding 通過", func(b *Binding) {}, ""},
		{"缺 contract_id 時被拒", func(b *Binding) { b.ContractID = "" }, "contract_id is required"},
		{"contract_id 只有空白時被拒", func(b *Binding) { b.ContractID = "   " }, "contract_id is required"},
		{"缺 contract_version 時被拒", func(b *Binding) { b.ContractVersion = "" }, "contract_version is required"},
		{"完全沒有 transport block 時被拒", func(b *Binding) { b.Proto = nil }, "at least one transport"},
		{"proto.service 缺時被拒", func(b *Binding) { b.Proto.Service = "" }, "service is required"},
		{"proto.method 缺時被拒", func(b *Binding) { b.Proto.Method = "" }, "method is required"},
		{"proto.file 缺時被拒", func(b *Binding) { b.Proto.File = "" }, "file is required"},
		{"proto.request 缺時被拒", func(b *Binding) { b.Proto.Request = "" }, "request is required"},
		{"proto.response 缺時被拒", func(b *Binding) { b.Proto.Response = "" }, "response is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := cloneBinding(valid)
			tt.mutate(b)
			err := ValidateBinding(b)
			if tt.wantErrMsg == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErrMsg)
			}
			if !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErrMsg)
			}
		})
	}

	t.Run("nil binding 時被拒", func(t *testing.T) {
		if err := ValidateBinding(nil); err == nil {
			t.Fatal("expected error for nil binding")
		}
	})

	t.Run("多欄位缺失時一次回報全部", func(t *testing.T) {
		b := &Binding{
			Proto: &ProtoBinding{
				// Method / File / Request / Response 全缺，只填 Service
				Service: "X",
			},
		}
		err := ValidateBinding(b)
		if err == nil {
			t.Fatal("expected error")
		}
		for _, want := range []string{"contract_id", "contract_version", "method", "file", "request", "response"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q missing %q", err.Error(), want)
			}
		}
	})
}

func TestReadBinding(t *testing.T) {
	t.Run("檔案不存在時回傳 ErrBindingNotFound", func(t *testing.T) {
		dir := t.TempDir()
		_, err := ReadBinding(dir, "missing-contract")
		if !errors.Is(err, ErrBindingNotFound) {
			t.Fatalf("expected ErrBindingNotFound, got %v", err)
		}
	})

	t.Run("正常讀取 YAML 檔", func(t *testing.T) {
		dir := t.TempDir()
		bindingsDir := BindingsDir(dir)
		if err := os.MkdirAll(bindingsDir, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		yaml := `contract_id: dungeon-session-list
contract_version: "1.0"
proto:
  service: SessionService
  method: List
  file: proto/sessions.proto
  request: ListSessionsRequest
  response: ListSessionsResponse
`
		if err := os.WriteFile(filepath.Join(bindingsDir, "dungeon-session-list.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatalf("setup write: %v", err)
		}

		b, err := ReadBinding(dir, "dungeon-session-list")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.ContractID != "dungeon-session-list" {
			t.Errorf("ContractID = %q, want dungeon-session-list", b.ContractID)
		}
		if b.Proto == nil || b.Proto.Service != "SessionService" {
			t.Errorf("Proto.Service mismatch: %+v", b.Proto)
		}
	})
}

func TestWriteBinding_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	b := &Binding{
		ContractID:      "dungeon-session-list",
		ContractVersion: "1.0",
		BoundAt:         "2026-04-21",
		Proto: &ProtoBinding{
			Service:  "SessionService",
			Method:   "List",
			File:     "proto/sessions.proto",
			Request:  "ListSessionsRequest",
			Response: "ListSessionsResponse",
		},
	}

	if err := WriteBinding(dir, b); err != nil {
		t.Fatalf("WriteBinding: %v", err)
	}

	got, err := ReadBinding(dir, "dungeon-session-list")
	if err != nil {
		t.Fatalf("ReadBinding: %v", err)
	}
	if got.ContractID != b.ContractID || got.Proto.Service != b.Proto.Service {
		t.Fatalf("round-trip mismatch: got %+v", got)
	}
}

func TestWriteBinding_RejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	b := &Binding{ContractID: "x", ContractVersion: "1.0"}
	if err := WriteBinding(dir, b); err == nil {
		t.Fatal("expected validation error, got nil")
	}
	// file should not exist since validation failed before write
	path, _ := BindingPath(dir, "x")
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("binding file should not exist after rejected write: %s", path)
	}
}

func TestDeleteBinding(t *testing.T) {
	t.Run("刪除存在的 binding", func(t *testing.T) {
		dir := t.TempDir()
		b := validBinding()
		if err := WriteBinding(dir, b); err != nil {
			t.Fatalf("setup write: %v", err)
		}
		if err := DeleteBinding(dir, b.ContractID); err != nil {
			t.Fatalf("DeleteBinding: %v", err)
		}
		if _, err := ReadBinding(dir, b.ContractID); !errors.Is(err, ErrBindingNotFound) {
			t.Fatalf("expected ErrBindingNotFound after delete, got %v", err)
		}
	})

	t.Run("刪除不存在的 binding 回傳 ErrBindingNotFound", func(t *testing.T) {
		dir := t.TempDir()
		if err := DeleteBinding(dir, "never-existed"); !errors.Is(err, ErrBindingNotFound) {
			t.Fatalf("expected ErrBindingNotFound, got %v", err)
		}
	})
}

func TestListBindings(t *testing.T) {
	t.Run("空目錄回傳空 slice", func(t *testing.T) {
		dir := t.TempDir()
		ids, err := ListBindings(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 0 {
			t.Fatalf("expected empty, got %v", ids)
		}
	})

	t.Run("按字母序列出所有 .yaml", func(t *testing.T) {
		dir := t.TempDir()
		bindingsDir := BindingsDir(dir)
		if err := os.MkdirAll(bindingsDir, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		// Create in non-sorted order, plus a non-yaml file that should be ignored.
		for _, name := range []string{"zeta.yaml", "alpha.yaml", "beta.yaml", "ignore.txt"} {
			if err := os.WriteFile(filepath.Join(bindingsDir, name), []byte("x"), 0o644); err != nil {
				t.Fatalf("setup write %s: %v", name, err)
			}
		}

		ids, err := ListBindings(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"alpha", "beta", "zeta"}
		if len(ids) != len(want) {
			t.Fatalf("got %v, want %v", ids, want)
		}
		for i, id := range ids {
			if id != want[i] {
				t.Fatalf("at %d: got %q, want %q", i, id, want[i])
			}
		}
	})
}

// --- helpers ---

func validBinding() *Binding {
	return &Binding{
		ContractID:      "dungeon-session-list",
		ContractVersion: "1.0",
		Proto: &ProtoBinding{
			Service:  "SessionService",
			Method:   "List",
			File:     "proto/sessions.proto",
			Request:  "ListSessionsRequest",
			Response: "ListSessionsResponse",
		},
	}
}

func cloneBinding(b *Binding) *Binding {
	out := *b
	if b.Proto != nil {
		p := *b.Proto
		out.Proto = &p
	}
	return &out
}

