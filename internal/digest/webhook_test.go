package digest

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		sec  int
		want string
	}{
		{0, "0s"},
		{30, "30s"},
		{60, "1m"},
		{90, "1m"},
		{3600, "1h 0m"},
		{3660, "1h 1m"},
		{7200, "2h 0m"},
		{5400, "1h 30m"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.sec); got != tt.want {
			t.Errorf("FormatDuration(%d) = %q, want %q", tt.sec, got, tt.want)
		}
	}
}

func TestFormatDiscord_使用AI摘要(t *testing.T) {
	d := &Digest{
		Date:         "2026-03-23",
		TotalWorkSec: 5400,
		Projects: []ProjectDigest{
			{
				ProjectName:  "piper",
				TotalWorkSec: 3600,
				Summary:      "完成 Digest v2 改版，支援 AI 歸納與 git commits 整合。",
				Sessions: []SessionDigest{
					{Name: "ragent-piper-abc", Description: "Daily Digest 功能"},
					{Name: "ragent-piper-def", Description: "修復 bug"},
				},
			},
			{
				ProjectName:  "myapp",
				TotalWorkSec: 1800,
				Summary:      "研究 OAuth 整合方案，尚未有具體產出。",
			},
		},
	}

	payload := FormatDiscord(d)

	if len(payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(payload.Embeds))
	}

	embed := payload.Embeds[0]
	if !strings.Contains(embed.Title, "2026-03-23") {
		t.Errorf("title missing date: %q", embed.Title)
	}
	if !strings.Contains(embed.Description, "1h 30m") {
		t.Errorf("description missing work time: %q", embed.Description)
	}
	// No completions in description
	if strings.Contains(embed.Description, "Completions") {
		t.Errorf("description should not contain completions: %q", embed.Description)
	}

	if len(embed.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(embed.Fields))
	}

	// First field: piper with AI summary (not session list)
	if !strings.Contains(embed.Fields[0].Name, "piper") {
		t.Errorf("field[0] name = %q, want piper", embed.Fields[0].Name)
	}
	if !strings.Contains(embed.Fields[0].Value, "Digest v2") {
		t.Errorf("field[0] should use AI summary: %q", embed.Fields[0].Value)
	}
	// Should NOT contain session descriptions when summary is present
	if strings.Contains(embed.Fields[0].Value, "Daily Digest 功能") {
		t.Errorf("field[0] should use summary, not session list: %q", embed.Fields[0].Value)
	}
}

func TestFormatDiscord_無摘要時Fallback到Session列表(t *testing.T) {
	d := &Digest{
		Date:         "2026-03-23",
		TotalWorkSec: 1800,
		Projects: []ProjectDigest{
			{
				ProjectName:  "myapp",
				TotalWorkSec: 1800,
				Sessions: []SessionDigest{
					{Name: "ragent-myapp-xyz", Description: ""},
				},
			},
		},
	}

	payload := FormatDiscord(d)
	embed := payload.Embeds[0]

	// Should show session name when no description and no summary
	if !strings.Contains(embed.Fields[0].Value, "ragent-myapp-xyz") {
		t.Errorf("field[0] should show session name: %q", embed.Fields[0].Value)
	}
}

func TestFormatDiscord_空digest顯示無活動(t *testing.T) {
	d := &Digest{
		Date:     "2026-03-23",
		Projects: []ProjectDigest{},
	}
	payload := FormatDiscord(d)
	if !strings.Contains(payload.Embeds[0].Description, "No activity") {
		t.Errorf("empty digest should mention no activity: %q", payload.Embeds[0].Description)
	}
}

// fakeHTTPClient captures the request for inspection.
type fakeHTTPClient struct {
	statusCode int
	lastBody   []byte
}

func (f *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	f.lastBody = body
	return &http.Response{
		StatusCode: f.statusCode,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func TestSendWebhook_成功發送(t *testing.T) {
	client := &fakeHTTPClient{statusCode: 204}
	d := &Digest{Date: "2026-03-23", Projects: []ProjectDigest{}}

	err := SendWebhook(client, "https://discord.com/api/webhooks/test", d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify payload was sent
	var payload discordWebhookPayload
	if err := json.Unmarshal(client.lastBody, &payload); err != nil {
		t.Fatalf("invalid payload JSON: %v", err)
	}
	if len(payload.Embeds) == 0 {
		t.Error("expected at least 1 embed in payload")
	}
}

func TestSendWebhook_非2xx回傳錯誤(t *testing.T) {
	client := &fakeHTTPClient{statusCode: 429}
	d := &Digest{Date: "2026-03-23", Projects: []ProjectDigest{}}

	err := SendWebhook(client, "https://discord.com/api/webhooks/test", d)
	if err == nil {
		t.Fatal("expected error for 429 status")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention status code: %v", err)
	}
}
