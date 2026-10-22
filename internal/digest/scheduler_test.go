package digest

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler_到達sendHour時發送(t *testing.T) {
	var sent atomic.Int32
	client := &fakeHTTPClient{statusCode: 204}

	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	s := NewScheduler(
		func(date string) *Digest {
			if date != yesterday {
				t.Errorf("expected date %s, got %s", yesterday, date)
			}
			sent.Add(1)
			return &Digest{Date: date, Projects: []ProjectDigest{}}
		},
		func() (string, bool, int, string) {
			return "https://hooks.example.com/test", true, now.Hour(), ""
		},
	)
	s.client = client

	// Manually trigger check
	s.check()

	if sent.Load() != 1 {
		t.Errorf("expected 1 send, got %d", sent.Load())
	}

	// Second check same day should not send again
	s.check()
	if sent.Load() != 1 {
		t.Errorf("expected still 1 send after dedup, got %d", sent.Load())
	}
}

func TestScheduler_未啟用時不發送(t *testing.T) {
	var sent atomic.Int32

	s := NewScheduler(
		func(date string) *Digest {
			sent.Add(1)
			return &Digest{Date: date}
		},
		func() (string, bool, int, string) {
			return "https://hooks.example.com/test", false, time.Now().UTC().Hour(), ""
		},
	)

	s.check()
	if sent.Load() != 0 {
		t.Errorf("should not send when disabled, got %d sends", sent.Load())
	}
}

func TestScheduler_非sendHour不發送(t *testing.T) {
	var sent atomic.Int32

	s := NewScheduler(
		func(date string) *Digest {
			sent.Add(1)
			return &Digest{Date: date}
		},
		func() (string, bool, int, string) {
			wrongHour := (time.Now().UTC().Hour() + 12) % 24
			return "https://hooks.example.com/test", true, wrongHour, ""
		},
	)

	s.check()
	if sent.Load() != 0 {
		t.Errorf("should not send at wrong hour, got %d sends", sent.Load())
	}
}

func TestScheduler_webhook失敗後重試(t *testing.T) {
	callCount := 0
	failClient := &countingHTTPClient{responses: []int{500, 204}}

	s := NewScheduler(
		func(date string) *Digest {
			callCount++
			return &Digest{Date: date, Projects: []ProjectDigest{}}
		},
		func() (string, bool, int, string) {
			return "https://hooks.example.com/test", true, time.Now().UTC().Hour(), ""
		},
	)
	s.client = failClient

	// First check: webhook fails
	s.check()
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second check: should retry (lastSent was reset)
	s.check()
	if callCount != 2 {
		t.Errorf("expected 2 calls after retry, got %d", callCount)
	}
}

func TestScheduler_時區修正日期_AsiaTaipei(t *testing.T) {
	// 模擬 Asia/Taipei (UTC+8)：sendHour=23 (UTC 23:00 = 本地隔天 07:00)
	// UTC 時間 2026-03-31 23:30 → 本地 2026-04-01 07:30
	// 期望摘要日期 = 本地的「昨天」= 2026-03-31，不是 UTC 的「昨天」= 2026-03-30
	var gotDate string
	client := &fakeHTTPClient{statusCode: 204}

	s := NewScheduler(
		func(date string) *Digest {
			gotDate = date
			return &Digest{Date: date, Projects: []ProjectDigest{}}
		},
		func() (string, bool, int, string) {
			return "https://hooks.example.com/test", true, 23, "Asia/Taipei"
		},
	)
	s.client = client
	s.nowFn = func() time.Time {
		return time.Date(2026, 3, 31, 23, 30, 0, 0, time.UTC)
	}

	s.check()

	if gotDate != "2026-03-31" {
		t.Errorf("expected digest date 2026-03-31 (local yesterday), got %s", gotDate)
	}
}

func TestScheduler_時區修正日期_AmericaNewYork(t *testing.T) {
	// 模擬 America/New_York (UTC-4 EDT)：sendHour=13 (UTC 13:00 = 本地 09:00)
	// UTC 時間 2026-07-15 13:15 → 本地 2026-07-15 09:15 (EDT)
	// 期望摘要日期 = 本地的「昨天」= 2026-07-14
	var gotDate string
	client := &fakeHTTPClient{statusCode: 204}

	s := NewScheduler(
		func(date string) *Digest {
			gotDate = date
			return &Digest{Date: date, Projects: []ProjectDigest{}}
		},
		func() (string, bool, int, string) {
			return "https://hooks.example.com/test", true, 13, "America/New_York"
		},
	)
	s.client = client
	s.nowFn = func() time.Time {
		return time.Date(2026, 7, 15, 13, 15, 0, 0, time.UTC)
	}

	s.check()

	if gotDate != "2026-07-14" {
		t.Errorf("expected digest date 2026-07-14 (local yesterday), got %s", gotDate)
	}
}

func TestScheduler_空時區預設UTC(t *testing.T) {
	// timezone 為空時，行為和之前一樣：UTC 的昨天
	var gotDate string
	client := &fakeHTTPClient{statusCode: 204}

	s := NewScheduler(
		func(date string) *Digest {
			gotDate = date
			return &Digest{Date: date, Projects: []ProjectDigest{}}
		},
		func() (string, bool, int, string) {
			return "https://hooks.example.com/test", true, 7, ""
		},
	)
	s.client = client
	s.nowFn = func() time.Time {
		return time.Date(2026, 4, 1, 7, 15, 0, 0, time.UTC)
	}

	s.check()

	if gotDate != "2026-03-31" {
		t.Errorf("expected digest date 2026-03-31, got %s", gotDate)
	}
}

// countingHTTPClient returns status codes in sequence.
type countingHTTPClient struct {
	responses []int
	idx       int
}

func (c *countingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	code := 200
	if c.idx < len(c.responses) {
		code = c.responses[c.idx]
		c.idx++
	}
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}
