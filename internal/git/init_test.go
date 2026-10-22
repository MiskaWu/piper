package git

import (
	"errors"
	"strings"
	"testing"
)

func TestIsRepo_是Git目錄(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /repo rev-parse --git-dir", ".git", nil)
	fetcher := NewGraphFetcher(mock)
	if !fetcher.IsRepo("/repo") {
		t.Error("expected IsRepo = true")
	}
}

func TestIsRepo_非Git目錄(t *testing.T) {
	mock := newGraphMock()
	mock.setOutput("git -C /notrepo rev-parse --git-dir", "", errors.New("not a git repository"))
	fetcher := NewGraphFetcher(mock)
	if fetcher.IsRepo("/notrepo") {
		t.Error("expected IsRepo = false")
	}
}

// initMockCommander records Run calls for verification.
type initMockCommander struct {
	runCalls []string
	runErr   error
}

func (m *initMockCommander) Output(name string, args ...string) ([]byte, error) {
	return nil, nil
}

func (m *initMockCommander) Run(name string, args ...string) error {
	m.runCalls = append(m.runCalls, name+" "+strings.Join(args, " "))
	return m.runErr
}

func TestInitRepo_成功初始化(t *testing.T) {
	mock := &initMockCommander{}
	fetcher := NewGraphFetcher(mock)
	err := fetcher.InitRepo("/newrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.runCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.runCalls))
	}
	expected := "git -C /newrepo init -b main"
	if mock.runCalls[0] != expected {
		t.Errorf("expected %q, got %q", expected, mock.runCalls[0])
	}
}

func TestInitRepo_失敗回傳錯誤(t *testing.T) {
	mock := &initMockCommander{runErr: errors.New("permission denied")}
	fetcher := NewGraphFetcher(mock)
	err := fetcher.InitRepo("/readonly")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should wrap original: %v", err)
	}
}

func TestAddRemote_成功加遠端(t *testing.T) {
	mock := &initMockCommander{}
	fetcher := NewGraphFetcher(mock)
	err := fetcher.AddRemote("/repo", "origin", "https://example.com/repo.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.runCalls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.runCalls))
	}
	call := mock.runCalls[0]
	if !strings.Contains(call, "remote add origin https://example.com/repo.git") {
		t.Errorf("unexpected call: %q", call)
	}
}

func TestAddRemote_失敗回傳錯誤(t *testing.T) {
	mock := &initMockCommander{runErr: errors.New("remote already exists")}
	fetcher := NewGraphFetcher(mock)
	err := fetcher.AddRemote("/repo", "origin", "url")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "remote already exists") {
		t.Errorf("error should wrap original: %v", err)
	}
}
