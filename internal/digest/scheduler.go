package digest

import (
	"log"
	"net/http"
	"sync"
	"time"
)

// DigestSource generates a Digest for the given date.
type DigestSource func(date string) *Digest

// ConfigSource returns the current digest configuration.
type ConfigSource func() (webhookURL string, enabled bool, sendHour int, timezone string)

// Scheduler periodically checks if it's time to send the daily digest.
type Scheduler struct {
	digestFn DigestSource
	configFn ConfigSource
	client   HTTPClient
	nowFn    func() time.Time // for testing; defaults to time.Now().UTC()

	mu       sync.Mutex
	lastSent string // date string of last sent digest (prevents duplicates)
	stopCh   chan struct{}
	stopped  sync.Once
}

// NewScheduler creates a digest scheduler.
func NewScheduler(digestFn DigestSource, configFn ConfigSource) *Scheduler {
	return &Scheduler{
		digestFn: digestFn,
		configFn: configFn,
		client:   http.DefaultClient,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the scheduler loop. Checks every 30 minutes.
func (s *Scheduler) Start() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	// Check immediately on start
	s.check()

	for {
		select {
		case <-ticker.C:
			s.check()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now().UTC()
}

// Stop halts the scheduler.
func (s *Scheduler) Stop() {
	s.stopped.Do(func() {
		close(s.stopCh)
	})
}

func (s *Scheduler) check() {
	webhookURL, enabled, sendHour, timezone := s.configFn()
	if !enabled || webhookURL == "" {
		return
	}

	now := s.now()
	if now.Hour() != sendHour {
		return
	}

	// Calculate "yesterday" in the user's local timezone.
	// sendHour is in UTC, but the target date should reflect the user's local date.
	// e.g. Asia/Taipei (UTC+8), sendHour=23: UTC 23:00 Mar 31 = local 07:00 Apr 1 → yesterday = Mar 31
	loc := time.UTC
	if timezone != "" {
		if l, err := time.LoadLocation(timezone); err == nil {
			loc = l
		}
	}
	yesterday := now.In(loc).AddDate(0, 0, -1).Format("2006-01-02")

	s.mu.Lock()
	if s.lastSent == yesterday {
		s.mu.Unlock()
		return
	}
	s.lastSent = yesterday
	s.mu.Unlock()

	d := s.digestFn(yesterday)
	if err := SendWebhook(s.client, webhookURL, d); err != nil {
		log.Printf("WARNING: digest webhook failed: %v", err)
		// Reset lastSent so it retries next check
		s.mu.Lock()
		s.lastSent = ""
		s.mu.Unlock()
		return
	}

	log.Printf("digest: sent daily digest for %s to webhook", yesterday)
}
