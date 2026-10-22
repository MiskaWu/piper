package digest

import (
	"testing"
	"time"

	"github.com/miskawu/piper/internal/usage"
)

func date(s string) time.Time {
	t, _ := time.Parse("2006-01-02T15:04:05Z", s)
	return t
}

func TestGenerate_BasicAggregation(t *testing.T) {
	targetDate := "2026-03-23"

	src := &Sources{
		WorkTimeEntries: []usage.WorkTimeEntry{
			{Timestamp: date("2026-03-23T08:00:00Z"), SessionName: "ragent-piper-abc123", DurationSec: 3600},
			{Timestamp: date("2026-03-23T10:00:00Z"), SessionName: "ragent-piper-abc123", DurationSec: 1800},
			{Timestamp: date("2026-03-23T14:00:00Z"), SessionName: "ragent-myapp-def456", DurationSec: 900},
			// different day — should be excluded
			{Timestamp: date("2026-03-22T09:00:00Z"), SessionName: "ragent-piper-abc123", DurationSec: 600},
		},
		SessionDescriptions: map[string]SessionInfo{
			"ragent-piper-abc123": {Description: "新增 Daily Digest 功能", ProjectPath: "/home/user/piper"},
			"ragent-myapp-def456": {Description: "實作用戶 API", ProjectPath: "/home/user/my-app"},
		},
		ProjectCommits: map[string][]string{
			"/home/user/piper": {"feat: 新增 digest 功能", "fix: 修復格式"},
		},
		ProjectConversations: map[string][]string{
			"/home/user/piper": {"實作 Daily Digest", "修復 webhook 格式"},
		},
	}

	d := Generate(targetDate, src, nil)

	if d.Date != targetDate {
		t.Errorf("Date = %q, want %q", d.Date, targetDate)
	}

	// Total work time: 3600 + 1800 + 900 = 6300
	if d.TotalWorkSec != 6300 {
		t.Errorf("TotalWorkSec = %d, want 6300", d.TotalWorkSec)
	}

	// Projects: 2 groups
	if len(d.Projects) != 2 {
		t.Fatalf("len(Projects) = %d, want 2", len(d.Projects))
	}

	// Find piper project
	var piper *ProjectDigest
	for i := range d.Projects {
		if d.Projects[i].ProjectPath == "/home/user/piper" {
			piper = &d.Projects[i]
		}
	}
	if piper == nil {
		t.Fatal("piper project not found")
	}
	if piper.ProjectName != "piper" {
		t.Errorf("piper ProjectName = %q, want %q", piper.ProjectName, "piper")
	}
	if len(piper.Sessions) != 1 {
		t.Fatalf("piper sessions = %d, want 1", len(piper.Sessions))
	}
	if piper.Sessions[0].WorkSec != 5400 {
		t.Errorf("piper session WorkSec = %d, want 5400", piper.Sessions[0].WorkSec)
	}
	if piper.Sessions[0].Description != "新增 Daily Digest 功能" {
		t.Errorf("piper session Description = %q", piper.Sessions[0].Description)
	}
	// Commits and conversations attached
	if len(piper.Commits) != 2 {
		t.Errorf("piper Commits = %d, want 2", len(piper.Commits))
	}
	if len(piper.Conversations) != 2 {
		t.Errorf("piper Conversations = %d, want 2", len(piper.Conversations))
	}
}

func TestGenerate_EmptySources(t *testing.T) {
	d := Generate("2026-03-23", &Sources{}, nil)

	if d.Date != "2026-03-23" {
		t.Errorf("Date = %q", d.Date)
	}
	if d.TotalWorkSec != 0 {
		t.Errorf("TotalWorkSec = %d", d.TotalWorkSec)
	}
	if len(d.Projects) != 0 {
		t.Errorf("len(Projects) = %d", len(d.Projects))
	}
}

func TestGenerate_SessionWithoutDescription(t *testing.T) {
	src := &Sources{
		WorkTimeEntries: []usage.WorkTimeEntry{
			{Timestamp: date("2026-03-23T08:00:00Z"), SessionName: "ragent-unknown-xyz", DurationSec: 120},
		},
		SessionDescriptions: map[string]SessionInfo{},
	}

	d := Generate("2026-03-23", src, nil)

	if len(d.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(d.Projects))
	}
	// Sessions without metadata go to empty-project group
	if d.Projects[0].ProjectPath != "" {
		t.Errorf("ProjectPath = %q, want empty", d.Projects[0].ProjectPath)
	}
	if d.Projects[0].Sessions[0].Name != "ragent-unknown-xyz" {
		t.Errorf("Session name = %q", d.Projects[0].Sessions[0].Name)
	}
}

func TestGenerate_優先使用Entry的ProjectPath(t *testing.T) {
	src := &Sources{
		WorkTimeEntries: []usage.WorkTimeEntry{
			{
				Timestamp:   date("2026-03-23T08:00:00Z"),
				SessionName: "ragent-piper-abc123",
				DurationSec: 3600,
				ProjectPath: "/home/user/piper",
			},
		},
		SessionDescriptions: map[string]SessionInfo{},
	}

	d := Generate("2026-03-23", src, nil)

	if len(d.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(d.Projects))
	}
	if d.Projects[0].ProjectPath != "/home/user/piper" {
		t.Errorf("ProjectPath = %q, want %q", d.Projects[0].ProjectPath, "/home/user/piper")
	}
	if d.Projects[0].ProjectName != "piper" {
		t.Errorf("ProjectName = %q, want %q", d.Projects[0].ProjectName, "piper")
	}
}

func TestGenerate_Fallback到SessionDescriptions(t *testing.T) {
	src := &Sources{
		WorkTimeEntries: []usage.WorkTimeEntry{
			{
				Timestamp:   date("2026-03-23T08:00:00Z"),
				SessionName: "ragent-old-abc123",
				DurationSec: 1800,
			},
		},
		SessionDescriptions: map[string]SessionInfo{
			"ragent-old-abc123": {Description: "舊格式 session", ProjectPath: "/home/user/old-proj"},
		},
	}

	d := Generate("2026-03-23", src, nil)

	if len(d.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(d.Projects))
	}
	if d.Projects[0].ProjectPath != "/home/user/old-proj" {
		t.Errorf("ProjectPath = %q, want %q", d.Projects[0].ProjectPath, "/home/user/old-proj")
	}
}

func TestGenerate_SortProjectsByWorkTime(t *testing.T) {
	src := &Sources{
		WorkTimeEntries: []usage.WorkTimeEntry{
			{Timestamp: date("2026-03-23T08:00:00Z"), SessionName: "ragent-small-aaa", DurationSec: 100},
			{Timestamp: date("2026-03-23T08:00:00Z"), SessionName: "ragent-big-bbb", DurationSec: 9000},
		},
		SessionDescriptions: map[string]SessionInfo{
			"ragent-small-aaa": {ProjectPath: "/a"},
			"ragent-big-bbb":   {ProjectPath: "/b"},
		},
	}

	d := Generate("2026-03-23", src, nil)

	if len(d.Projects) != 2 {
		t.Fatalf("len(Projects) = %d, want 2", len(d.Projects))
	}
	// Highest work time first
	if d.Projects[0].TotalWorkSec != 9000 {
		t.Errorf("First project WorkSec = %d, want 9000", d.Projects[0].TotalWorkSec)
	}
}

func TestGenerate_CommitsAndConversationsAttached(t *testing.T) {
	src := &Sources{
		WorkTimeEntries: []usage.WorkTimeEntry{
			{Timestamp: date("2026-03-23T08:00:00Z"), SessionName: "ragent-app-abc", DurationSec: 3600, ProjectPath: "/proj"},
		},
		SessionDescriptions: map[string]SessionInfo{},
		ProjectCommits: map[string][]string{
			"/proj": {"feat: 新功能", "refactor: 重構"},
		},
		ProjectConversations: map[string][]string{
			"/proj": {"實作新功能", "討論架構"},
		},
	}

	d := Generate("2026-03-23", src, nil)

	if len(d.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(d.Projects))
	}
	if len(d.Projects[0].Commits) != 2 {
		t.Errorf("Commits = %d, want 2", len(d.Projects[0].Commits))
	}
	if len(d.Projects[0].Conversations) != 2 {
		t.Errorf("Conversations = %d, want 2", len(d.Projects[0].Conversations))
	}
}

func TestGenerate_時區感知_台北晚間工作歸到當天(t *testing.T) {
	taipei, _ := time.LoadLocation("Asia/Taipei")

	// 台北 2026-04-06 22:00 = UTC 2026-04-06 14:00 → 應歸到 4/6
	// 台北 2026-04-07 01:00 = UTC 2026-04-06 17:00 → 用 UTC 會歸到 4/6，用台北應歸到 4/7
	src := &Sources{
		WorkTimeEntries: []usage.WorkTimeEntry{
			{Timestamp: date("2026-04-06T14:00:00Z"), SessionName: "ragent-a-1", DurationSec: 3600, ProjectPath: "/proj"},
			{Timestamp: date("2026-04-06T17:00:00Z"), SessionName: "ragent-a-2", DurationSec: 1800, ProjectPath: "/proj"},
		},
		SessionDescriptions: map[string]SessionInfo{},
	}

	// 查 4/6（台北時區）：只有 ragent-a-1（UTC 14:00 = 台北 22:00 4/6）
	d := Generate("2026-04-06", src, taipei)
	if d.TotalWorkSec != 3600 {
		t.Errorf("TotalWorkSec for 4/6 (Taipei) = %d, want 3600", d.TotalWorkSec)
	}

	// 查 4/7（台北時區）：只有 ragent-a-2（UTC 17:00 = 台北 01:00 4/7）
	d2 := Generate("2026-04-07", src, taipei)
	if d2.TotalWorkSec != 1800 {
		t.Errorf("TotalWorkSec for 4/7 (Taipei) = %d, want 1800", d2.TotalWorkSec)
	}

	// 對比：用 UTC 查 4/6 應該兩筆都算進去
	d3 := Generate("2026-04-06", src, nil)
	if d3.TotalWorkSec != 5400 {
		t.Errorf("TotalWorkSec for 4/6 (UTC) = %d, want 5400", d3.TotalWorkSec)
	}
}
