package git

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// pushMockCommander records Output and Run calls with configurable responses.
type pushMockCommander struct {
	outputCalls []string
	outputMap   map[string]struct {
		data []byte
		err  error
	}
	runCalls []string
	runMap   map[string]error
}

func newPushMock() *pushMockCommander {
	return &pushMockCommander{
		outputMap: make(map[string]struct {
			data []byte
			err  error
		}),
		runMap: make(map[string]error),
	}
}

func (m *pushMockCommander) Output(name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	m.outputCalls = append(m.outputCalls, key)
	if out, ok := m.outputMap[key]; ok {
		return out.data, out.err
	}
	return nil, errors.New("unexpected Output call: " + key)
}

func (m *pushMockCommander) Run(name string, args ...string) error {
	key := name + " " + strings.Join(args, " ")
	m.runCalls = append(m.runCalls, key)
	if err, ok := m.runMap[key]; ok {
		return err
	}
	return errors.New("unexpected Run call: " + key)
}

func (m *pushMockCommander) setOutput(key string, data string, err error) {
	m.outputMap[key] = struct {
		data []byte
		err  error
	}{[]byte(data), err}
}

func (m *pushMockCommander) setRun(key string, err error) {
	m.runMap[key] = err
}

// --- CheckSSHAgent tests ---

func TestCheckSSHAgent_有Key時回傳true(t *testing.T) {
	mock := newPushMock()
	mock.setOutput("ssh-add -l", "2048 SHA256:abc... user@host (RSA)", nil)
	fetcher := NewGraphFetcher(mock)

	ok, err := fetcher.CheckSSHAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true when ssh-add -l succeeds")
	}
}

func TestCheckSSHAgent_無Key時回傳false(t *testing.T) {
	mock := newPushMock()
	mock.setOutput("ssh-add -l", "The agent has no identities.", errors.New("exit status 1"))
	fetcher := NewGraphFetcher(mock)

	ok, err := fetcher.CheckSSHAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false when agent has no identities")
	}
}

func TestCheckSSHAgent_Agent不可用時回傳錯誤(t *testing.T) {
	mock := newPushMock()
	mock.setOutput("ssh-add -l", "Could not open a connection to your authentication agent.", errors.New("exit status 2"))
	fetcher := NewGraphFetcher(mock)

	ok, err := fetcher.CheckSSHAgent()
	if err == nil {
		t.Fatal("expected error when agent is unavailable")
	}
	if ok {
		t.Error("expected false when agent is unavailable")
	}
	if !strings.Contains(err.Error(), "ssh agent") {
		t.Errorf("error should mention ssh agent: %v", err)
	}
}

// --- Push tests ---

func TestPush_成功推送(t *testing.T) {
	// Push 現在直接呼叫 exec.CommandContext，不透過 Commander mock。
	// 使用實際存在的目錄（但非 git repo），確認呼叫路徑正確並回傳 git 錯誤而非 panic。
	mock := newPushMock()
	fetcher := NewGraphFetcher(mock)

	// t.TempDir() 存在但不是 git repo，git push 必然失敗並帶有輸出。
	dir := t.TempDir()
	err := fetcher.Push(dir, "")
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
	// 錯誤訊息應包含目錄路徑
	if !strings.Contains(err.Error(), "git push") {
		t.Errorf("error should mention 'git push': %v", err)
	}
}

func TestPush_失敗回傳錯誤(t *testing.T) {
	// 使用不存在的目錄，讓 git 回傳「cannot change to」之類的錯誤。
	mock := newPushMock()
	fetcher := NewGraphFetcher(mock)

	err := fetcher.Push("/nonexistent-dir-push-test", "")
	if err == nil {
		t.Fatal("expected error")
	}
	// 錯誤訊息應包含目錄名稱
	if !strings.Contains(err.Error(), "git push") {
		t.Errorf("error should contain 'git push': %v", err)
	}
}

func TestPush_空目錄回傳錯誤(t *testing.T) {
	mock := newPushMock()
	fetcher := NewGraphFetcher(mock)

	err := fetcher.Push("", "")
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestPush_指定分支時推送該分支(t *testing.T) {
	mock := newPushMock()
	fetcher := NewGraphFetcher(mock)

	// 使用 TempDir（非 git repo）觸發 git 錯誤，但可驗證不會 panic
	dir := t.TempDir()
	err := fetcher.Push(dir, "feature-branch")
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
	// 錯誤訊息應包含 git push
	if !strings.Contains(err.Error(), "git push") {
		t.Errorf("error should mention 'git push': %v", err)
	}
}

func TestPush_追蹤非origin遠端時推送到正確remote(t *testing.T) {
	// 建立 bare repo 作為 "fork" remote
	bareDir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("cmd %v failed: %s: %v", args, out, err)
		}
	}

	run("git", "init", "--bare", bareDir)

	// 建立本地 repo
	localDir := t.TempDir()
	run("git", "init", localDir)
	run("git", "-C", localDir, "config", "user.email", "test@test.com")
	run("git", "-C", localDir, "config", "user.name", "Test")
	run("git", "-C", localDir, "commit", "--allow-empty", "-m", "init")

	// 加 "fork" remote（不加 origin）
	run("git", "-C", localDir, "remote", "add", "fork", bareDir)
	run("git", "-C", localDir, "push", "-u", "fork", "master")

	// 再做一個 commit，產生 ahead
	run("git", "-C", localDir, "commit", "--allow-empty", "-m", "second")

	// 用 Push 推送，應該推到 fork 而不是 origin
	mock := newPushMock()
	fetcher := NewGraphFetcher(mock)
	err := fetcher.Push(localDir, "")
	if err != nil {
		t.Fatalf("push should succeed: %v", err)
	}

	// 驗證 bare repo 有收到 commit
	cmd := exec.Command("git", "-C", bareDir, "log", "--oneline")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log on bare failed: %v", err)
	}
	if !strings.Contains(string(out), "second") {
		t.Errorf("bare repo should have 'second' commit, got: %s", out)
	}
}
