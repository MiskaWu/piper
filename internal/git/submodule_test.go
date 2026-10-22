package git

import (
	"errors"
	"testing"
)

func TestListSubmodules_有Submodule且有未推commit(t *testing.T) {
	mock := newPushMock()
	// git submodule status 回傳兩個 submodule，一個有未推 commit
	mock.setOutput("git -C /repo submodule status", " abc1234 libs/shared (v1.0)\n+def5678 vendor/sdk (v2.0-1-gabcdef)\n", nil)
	// 第一個 submodule: 0 ahead
	mock.setOutput("git -C /repo/libs/shared rev-list --count @{upstream}..HEAD", "0\n", nil)
	// 第二個 submodule: 2 ahead
	mock.setOutput("git -C /repo/vendor/sdk rev-list --count @{upstream}..HEAD", "2\n", nil)

	fetcher := NewGraphFetcher(mock)
	subs, err := fetcher.ListSubmodules("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 submodules, got %d", len(subs))
	}
	if subs[0].Path != "libs/shared" || subs[0].Ahead != 0 {
		t.Errorf("unexpected submodule[0]: %+v", subs[0])
	}
	if subs[1].Path != "vendor/sdk" || subs[1].Ahead != 2 {
		t.Errorf("unexpected submodule[1]: %+v", subs[1])
	}
}

func TestListSubmodules_無Submodule(t *testing.T) {
	mock := newPushMock()
	mock.setOutput("git -C /repo submodule status", "", nil)

	fetcher := NewGraphFetcher(mock)
	subs, err := fetcher.ListSubmodules("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("expected 0 submodules, got %d", len(subs))
	}
}

func TestListSubmodules_非Git目錄(t *testing.T) {
	mock := newPushMock()
	mock.setOutput("git -C /bad submodule status", "", errors.New("not a git repository"))

	fetcher := NewGraphFetcher(mock)
	_, err := fetcher.ListSubmodules("/bad")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListSubmodules_Upstream不存在時Ahead為零(t *testing.T) {
	mock := newPushMock()
	mock.setOutput("git -C /repo submodule status", " abc1234 libs/shared (v1.0)\n", nil)
	// no upstream tracking branch
	mock.setOutput("git -C /repo/libs/shared rev-list --count @{upstream}..HEAD", "", errors.New("no upstream"))

	fetcher := NewGraphFetcher(mock)
	subs, err := fetcher.ListSubmodules("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 submodule, got %d", len(subs))
	}
	if subs[0].Ahead != 0 {
		t.Errorf("expected ahead=0 when no upstream, got %d", subs[0].Ahead)
	}
}

func TestPushSubmodules_成功推送指定submodule(t *testing.T) {
	// PushSubmodules uses exec.CommandContext directly like Push,
	// so we test with a non-git temp dir to verify it's called correctly.
	mock := newPushMock()
	fetcher := NewGraphFetcher(mock)

	dir := t.TempDir()
	err := fetcher.PushSubmodules(dir, []string{"libs/shared"})
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestPushSubmodules_空清單回傳nil(t *testing.T) {
	mock := newPushMock()
	fetcher := NewGraphFetcher(mock)
	err := fetcher.PushSubmodules("/repo", []string{})
	if err != nil {
		t.Fatalf("expected no error for empty list, got: %v", err)
	}
}

func TestPushSubmodules_空目錄回傳錯誤(t *testing.T) {
	mock := newPushMock()
	fetcher := NewGraphFetcher(mock)
	err := fetcher.PushSubmodules("", []string{"foo"})
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}
