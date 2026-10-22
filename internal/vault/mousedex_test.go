package vault

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/miskawu/piper/internal/mousedex"
	"github.com/miskawu/piper/internal/plan"
)

func intPtr(v int) *int { return &v }

// noopGitRunner for testing — does nothing
type noopGitRunner struct{}

func (g noopGitRunner) Init(dir string) error                     { return nil }
func (g noopGitRunner) IsRepo(dir string) bool                    { return false }
func (g noopGitRunner) Add(dir string, paths ...string) error     { return nil }
func (g noopGitRunner) Commit(dir string, msg string) error       { return nil }
func (g noopGitRunner) RemoteExists(dir, name string) bool        { return false }
func (g noopGitRunner) RemoteAdd(dir, name, url string) error     { return nil }
func (g noopGitRunner) RemoteSetURL(dir, name, url string) error  { return nil }
func (g noopGitRunner) LsRemote(dir, url string) error            { return nil }
func (g noopGitRunner) Pull(dir string) error                     { return nil }
func (g noopGitRunner) Push(dir string) error                     { return nil }
func (g noopGitRunner) PendingCommits(dir string) (int, error)    { return 0, nil }

var _ plan.GitRunner = noopGitRunner{}

func setupTestVault(t *testing.T) *VaultManager {
	t.Helper()
	dir := t.TempDir()
	v := New(dir, WithGitRunner(noopGitRunner{}))
	if err := v.Init(); err != nil {
		t.Fatalf("vault init: %v", err)
	}
	return v
}

func TestCollectMouse_新物種寫入(t *testing.T) {
	v := setupTestVault(t)
	cfg := mousedex.MouseConfig{
		IsSSR: false, BodyColor: "#F8C8C8", Color: "#FF6B6B",
		TopIdx: intPtr(1), MidIdx: nil, BotIdx: intPtr(3),
	}
	isNew, err := v.CollectMouse("ragent-test-001", cfg)
	if err != nil {
		t.Fatalf("CollectMouse: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true for first collection")
	}

	// 驗證檔案存在
	if _, err := os.Stat(filepath.Join(v.baseDir, "mousedex.json")); err != nil {
		t.Fatalf("mousedex.json not found: %v", err)
	}

	// 重新載入驗證
	mdex, err := v.LoadMousedex()
	if err != nil {
		t.Fatalf("LoadMousedex: %v", err)
	}
	if len(mdex.Collected) != 1 {
		t.Errorf("collected = %d, want 1", len(mdex.Collected))
	}
	entry, ok := mdex.Collected["1-x-3"]
	if !ok {
		t.Fatal("species key '1-x-3' not found")
	}
	if entry.SessionName != "ragent-test-001" {
		t.Errorf("sessionName = %q, want 'ragent-test-001'", entry.SessionName)
	}
	if entry.BodyColor != "#F8C8C8" {
		t.Errorf("bodyColor = %q, want '#F8C8C8'", entry.BodyColor)
	}
	if entry.UnlockCount != 1 {
		t.Errorf("unlockCount = %d, want 1", entry.UnlockCount)
	}
}

func TestCollectMouse_重複物種不覆蓋但遞增解鎖次數(t *testing.T) {
	v := setupTestVault(t)
	cfg := mousedex.MouseConfig{
		IsSSR: false, BodyColor: "#F8C8C8", Color: "#FF6B6B",
		TopIdx: nil, MidIdx: nil, BotIdx: nil,
	}
	isNew, _ := v.CollectMouse("first-session", cfg)
	if !isNew {
		t.Error("first collection should be new")
	}

	// 首次解鎖，UnlockCount 應為 1
	mdex, _ := v.LoadMousedex()
	entry := mdex.Collected["x-x-x"]
	if entry.UnlockCount != 1 {
		t.Errorf("first unlock: UnlockCount = %d, want 1", entry.UnlockCount)
	}

	// 同組合不同顏色
	cfg2 := mousedex.MouseConfig{
		IsSSR: false, BodyColor: "#BBDEFB", Color: "#4D96FF",
		TopIdx: nil, MidIdx: nil, BotIdx: nil,
	}
	isNew2, _ := v.CollectMouse("second-session", cfg2)
	if isNew2 {
		t.Error("same species key should not be new")
	}

	// 重複解鎖後 UnlockCount 應為 2，但保留首次資料
	mdex, _ = v.LoadMousedex()
	entry = mdex.Collected["x-x-x"]
	if entry.SessionName != "first-session" {
		t.Errorf("should keep first session name, got %q", entry.SessionName)
	}
	if entry.BodyColor != "#F8C8C8" {
		t.Errorf("should keep first body color, got %q", entry.BodyColor)
	}
	if entry.UnlockCount != 2 {
		t.Errorf("second unlock: UnlockCount = %d, want 2", entry.UnlockCount)
	}

	// 第三次解鎖
	v.CollectMouse("third-session", cfg)
	mdex, _ = v.LoadMousedex()
	entry = mdex.Collected["x-x-x"]
	if entry.UnlockCount != 3 {
		t.Errorf("third unlock: UnlockCount = %d, want 3", entry.UnlockCount)
	}
}

func TestCollectMouse_SSR物種(t *testing.T) {
	v := setupTestVault(t)
	cfg := mousedex.MouseConfig{
		IsSSR: true, BodyColor: "#F8C8C8", Color: "#FF6B6B",
	}
	isNew, err := v.CollectMouse("ssr-session", cfg)
	if err != nil {
		t.Fatalf("CollectMouse: %v", err)
	}
	if !isNew {
		t.Error("SSR should be new")
	}

	mdex, _ := v.LoadMousedex()
	if _, ok := mdex.Collected["ssr"]; !ok {
		t.Error("SSR species key not found")
	}
}

func TestGetMousedexStats(t *testing.T) {
	v := setupTestVault(t)

	stats, err := v.GetMousedexStats()
	if err != nil {
		t.Fatalf("GetMousedexStats: %v", err)
	}
	if stats.Collected != 0 || stats.Total != 1332 {
		t.Errorf("stats = %+v, want {0, 1332}", stats)
	}

	cfg := mousedex.MouseConfig{IsSSR: true}
	v.CollectMouse("test", cfg)

	stats, _ = v.GetMousedexStats()
	if stats.Collected != 1 {
		t.Errorf("collected = %d, want 1", stats.Collected)
	}
}

func TestLoadMousedex_檔案不存在回傳空集合(t *testing.T) {
	v := setupTestVault(t)
	mdex, err := v.LoadMousedex()
	if err != nil {
		t.Fatalf("LoadMousedex: %v", err)
	}
	if len(mdex.Collected) != 0 {
		t.Errorf("expected empty collection, got %d", len(mdex.Collected))
	}
}
