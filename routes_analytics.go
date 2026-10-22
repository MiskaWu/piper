package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	claudePkg "github.com/miskawu/piper/internal/claude"
	digestPkg "github.com/miskawu/piper/internal/digest"
	gitPkg "github.com/miskawu/piper/internal/git"
	"github.com/miskawu/piper/internal/store"
	"github.com/miskawu/piper/internal/summarizer"
	"github.com/miskawu/piper/internal/usage"
)

func (app *App) registerAnalyticsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/dashboard-stats", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		year := now.Year()
		month := now.Month()

		if q := r.URL.Query().Get("year"); q != "" {
			if v, err := strconv.Atoi(q); err == nil {
				year = v
			}
		}
		if q := r.URL.Query().Get("month"); q != "" {
			if v, err := strconv.Atoi(q); err == nil && v >= 1 && v <= 12 {
				month = time.Month(v)
			}
		}

		type dashboardUsage struct {
			*usage.UsageMonthlySummary
			Daily []usage.DailyAverage `json:"daily"`
		}
		type dashboardWorktime struct {
			*usage.WorkTimeSummary
			Daily          []usage.DailyWorkTime          `json:"daily"`
			DailySegmented []usage.DailySegmentedWorkTime `json:"dailySegmented"`
		}
		type dashboardCompletions struct {
			Daily []usage.DailyCompletion `json:"daily"`
		}
		type dashboardResponse struct {
			Month       string                      `json:"month"`
			Usage       *dashboardUsage             `json:"usage"`
			Worktime    *dashboardWorktime          `json:"worktime"`
			SummaryCost *summarizer.MonthlySummary  `json:"summaryCost"`
			Completions *dashboardCompletions       `json:"completions"`
		}

		monthStr := fmt.Sprintf("%04d-%02d", year, month)
		resp := dashboardResponse{Month: monthStr}

		if app.usageLogger != nil {
			if s, err := app.usageLogger.GetMonthlySummary(year, month); err == nil {
				du := &dashboardUsage{UsageMonthlySummary: s}
				if daily, err := app.usageLogger.GetDailyAverages(year, month); err == nil && daily != nil {
					du.Daily = daily
				} else {
					du.Daily = []usage.DailyAverage{}
				}
				resp.Usage = du
			}
		}
		if resp.Usage == nil {
			resp.Usage = &dashboardUsage{
				UsageMonthlySummary: &usage.UsageMonthlySummary{Month: monthStr},
				Daily:               []usage.DailyAverage{},
			}
		}

		if app.workTimeTracker != nil {
			if s, err := app.workTimeTracker.GetMonthlySummary(year, month); err == nil {
				dw := &dashboardWorktime{WorkTimeSummary: s}
				if daily, err := app.workTimeTracker.GetDailyWorkTime(year, month); err == nil && daily != nil {
					dw.Daily = daily
				} else {
					dw.Daily = []usage.DailyWorkTime{}
				}
				if seg, err := app.workTimeTracker.GetDailySegmentedWorkTime(year, month); err == nil && seg != nil {
					dw.DailySegmented = seg
				} else {
					dw.DailySegmented = []usage.DailySegmentedWorkTime{}
				}
				resp.Worktime = dw
			}
		}
		if resp.Worktime == nil {
			resp.Worktime = &dashboardWorktime{
				WorkTimeSummary: &usage.WorkTimeSummary{Month: monthStr},
				Daily:           []usage.DailyWorkTime{},
				DailySegmented:  []usage.DailySegmentedWorkTime{},
			}
		}

		if app.costLogger != nil {
			if s, err := app.costLogger.GetMonthlySummary(year, month); err == nil {
				resp.SummaryCost = &s
			}
		}
		if resp.SummaryCost == nil {
			resp.SummaryCost = &summarizer.MonthlySummary{Month: monthStr}
		}

		if app.completionTracker != nil {
			if daily, err := app.completionTracker.GetDailyCompletions(year, month); err == nil {
				resp.Completions = &dashboardCompletions{Daily: daily}
			}
		}
		if resp.Completions == nil {
			resp.Completions = &dashboardCompletions{
				Daily: []usage.DailyCompletion{},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("GET /api/summary-stats", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		year := now.Year()
		month := now.Month()

		if q := r.URL.Query().Get("year"); q != "" {
			if v, err := strconv.Atoi(q); err == nil {
				year = v
			}
		}
		if q := r.URL.Query().Get("month"); q != "" {
			if v, err := strconv.Atoi(q); err == nil && v >= 1 && v <= 12 {
				month = time.Month(v)
			}
		}

		if app.costLogger == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(summarizer.MonthlySummary{Month: fmt.Sprintf("%04d-%02d", year, month)})
			return
		}

		summary, err := app.costLogger.GetMonthlySummary(year, month)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get summary stats: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(summary)
	})

	// Helper to generate digest
	// DigestProgress callback for SSE streaming. nil means no progress reporting.
	type DigestProgress func(step, message string, current, total int)

	// resolveDigestLocation returns the *time.Location from digest config.
	resolveDigestLocation := func() *time.Location {
		if app.sessionStore == nil {
			return time.UTC
		}
		cfg := app.sessionStore.GetDigestConfig()
		if cfg.Timezone != "" {
			if loc, err := time.LoadLocation(cfg.Timezone); err == nil {
				return loc
			}
		}
		return time.UTC
	}

	generateDigestWithProgress := func(dateStr string, progress DigestProgress) *digestPkg.Digest {
		notify := func(step, message string, current, total int) {
			if progress != nil {
				progress(step, message, current, total)
			}
		}

		notify("collecting", "收集工作資料...", 0, 0)

		loc := resolveDigestLocation()
		targetDate, _ := time.Parse("2006-01-02", dateStr)
		year, month, _ := targetDate.Date()

		src := &digestPkg.Sources{
			SessionDescriptions:  map[string]digestPkg.SessionInfo{},
			ProjectCommits:       map[string][]string{},
			ProjectConversations: map[string][]string{},
		}

		if app.workTimeTracker != nil {
			if entries, err := app.workTimeTracker.GetMonthlyEntries(year, month); err == nil {
				src.WorkTimeEntries = entries
			}
		}
		if app.sessionStore != nil {
			// Snapshot active sessions so digest captures still-running work
			if tmuxSessions, err := app.tmuxClient.ListSessions(); err == nil {
				activeNames := make([]string, 0, len(tmuxSessions))
				for _, ts := range tmuxSessions {
					activeNames = append(activeNames, ts.Name)
				}
				app.sessionStore.SnapshotActiveSessions(activeNames)
			}
			// Read from history (closed + just-snapshotted active sessions)
			dayStart, _ := time.Parse("2006-01-02", dateStr)
			dayEnd := dayStart.AddDate(0, 0, 1).Add(-time.Second)
			if history, err := app.sessionStore.GetSessionHistory(dayStart, dayEnd); err == nil {
				for _, e := range history {
					src.SessionDescriptions[e.Name] = digestPkg.SessionInfo{
						Description: e.Description,
						ProjectPath: e.ProjectPath,
					}
				}
			}
		}

		// Collect unique project paths from work time entries
		projectPaths := map[string]struct{}{}
		for _, e := range src.WorkTimeEntries {
			if e.Timestamp.In(loc).Format("2006-01-02") == dateStr && e.ProjectPath != "" {
				projectPaths[e.ProjectPath] = struct{}{}
			}
		}
		// Also from session descriptions
		for _, info := range src.SessionDescriptions {
			if info.ProjectPath != "" {
				projectPaths[info.ProjectPath] = struct{}{}
			}
		}

		// Collect git commits per project
		nextDay := targetDate.AddDate(0, 0, 1).Format("2006-01-02")
		if app.enrichCmd != nil {
			gitChecker := gitPkg.NewChecker(app.enrichCmd)
			for projPath := range projectPaths {
				commits, err := gitChecker.Log(gitPkg.LogOptions{
					Dir:   projPath,
					Since: dateStr,
					Until: nextDay,
				})
				if err != nil || len(commits) == 0 {
					continue
				}
				subjects := make([]string, len(commits))
				for i, c := range commits {
					subjects[i] = c.Subject
				}
				src.ProjectCommits[projPath] = subjects
			}
		}

		// Collect conversation history
		home, _ := os.UserHomeDir()
		historyPath := filepath.Join(home, ".claude", "history.jsonl")
		if history, err := claudePkg.ReadDailyHistory(historyPath, dateStr, loc); err == nil {
			for projPath, entries := range history {
				if _, ok := projectPaths[projPath]; !ok {
					continue // only include projects with work time
				}
				prompts := make([]string, len(entries))
				for i, e := range entries {
					prompts[i] = e.Display
				}
				src.ProjectConversations[projPath] = prompts
			}
		}

		d := digestPkg.Generate(dateStr, src, loc)

		// AI summarization per project
		if app.enrichCmd != nil {
			summarizer := digestPkg.NewSummarizer(app.enrichCmd, "")
			// Count projects that need summarization
			needSummary := 0
			for i := range d.Projects {
				if len(d.Projects[i].Commits) > 0 || len(d.Projects[i].Conversations) > 0 {
					needSummary++
				}
			}
			current := 0
			for i := range d.Projects {
				proj := &d.Projects[i]
				if len(proj.Commits) == 0 && len(proj.Conversations) == 0 {
					continue
				}
				current++
				notify("summarizing", fmt.Sprintf("AI 摘要：%s", proj.ProjectName), current, needSummary)
				proj.Summary = summarizer.SummarizeProject(proj.ProjectName, proj.Commits, proj.Conversations)
			}
		}

		return d
	}

	generateDigest := func(dateStr string) *digestPkg.Digest {
		return generateDigestWithProgress(dateStr, nil)
	}

	mux.HandleFunc("GET /api/digest", func(w http.ResponseWriter, r *http.Request) {
		dateStr := r.URL.Query().Get("date")
		if dateStr == "" {
			dateStr = time.Now().In(resolveDigestLocation()).AddDate(0, 0, -1).Format("2006-01-02")
		}
		if _, err := time.Parse("2006-01-02", dateStr); err != nil {
			http.Error(w, "invalid date format, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}

		result := generateDigest(dateStr)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("GET /api/debug/digest", func(w http.ResponseWriter, r *http.Request) {
		dateStr := r.URL.Query().Get("date")
		if dateStr == "" {
			dateStr = time.Now().In(resolveDigestLocation()).AddDate(0, 0, -1).Format("2006-01-02")
		}
		targetDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Error(w, "invalid date format, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		year, month, _ := targetDate.Date()

		type fileInfo struct {
			Path        string `json:"path"`
			Exists      bool   `json:"exists"`
			Size        int64  `json:"size,omitempty"`
			EntryCount  int    `json:"entryCount"`
			MatchedDate int    `json:"matchedDate"`
		}

		checkFile := func(path string) fileInfo {
			fi := fileInfo{Path: path}
			info, err := os.Stat(path)
			if err == nil {
				fi.Exists = true
				fi.Size = info.Size()
			}
			return fi
		}

		debugLoc := resolveDigestLocation()
		files := map[string]fileInfo{}

		if app.workTimeTracker != nil {
			fi := checkFile(app.workTimeTracker.FilePath(year, month))
			if entries, err := app.workTimeTracker.GetMonthlyEntries(year, month); err == nil {
				fi.EntryCount = len(entries)
				for _, e := range entries {
					if e.Timestamp.In(debugLoc).Format("2006-01-02") == dateStr {
						fi.MatchedDate++
					}
				}
			}
			files["worktime"] = fi
		}

		if app.completionTracker != nil {
			fi := checkFile(app.completionTracker.FilePath(year, month))
			if entries, err := app.completionTracker.GetMonthlyEntries(year, month); err == nil {
				fi.EntryCount = len(entries)
				for _, e := range entries {
					if e.Timestamp.In(debugLoc).Format("2006-01-02") == dateStr {
						fi.MatchedDate++
					}
				}
			}
			files["completions"] = fi
		}

		if app.costLogger != nil {
			fi := checkFile(app.costLogger.FilePath(year, month))
			if entries, err := app.costLogger.GetMonthlyEntries(year, month); err == nil {
				fi.EntryCount = len(entries)
				for _, e := range entries {
					if e.Timestamp.In(debugLoc).Format("2006-01-02") == dateStr {
						fi.MatchedDate++
					}
				}
			}
			files["cost"] = fi
		}

		var activeTracking []string
		if app.workTimeTracker != nil {
			activeTracking = app.workTimeTracker.ActiveSessions()
		}
		if activeTracking == nil {
			activeTracking = []string{}
		}

		descriptions := map[string]digestPkg.SessionInfo{}
		if app.sessionStore != nil {
			dayStart, _ := time.Parse("2006-01-02", dateStr)
			dayEnd := dayStart.AddDate(0, 0, 1).Add(-time.Second)
			if history, err := app.sessionStore.GetSessionHistory(dayStart, dayEnd); err == nil {
				for _, e := range history {
					descriptions[e.Name] = digestPkg.SessionInfo{
						Description: e.Description,
						ProjectPath: e.ProjectPath,
					}
				}
			}
			// Also include active sessions (for today's stats)
			for name, meta := range app.sessionStore.AllSessions() {
				if _, exists := descriptions[name]; !exists {
					descriptions[name] = digestPkg.SessionInfo{
						Description: meta.Description,
						ProjectPath: meta.ProjectPath,
					}
				}
			}
		}

		var cfg store.DigestConfig
		if app.sessionStore != nil {
			cfg = app.sessionStore.GetDigestConfig()
		}

		d := generateDigest(dateStr)

		result := map[string]interface{}{
			"targetDate":          dateStr,
			"files":               files,
			"activeTracking":      activeTracking,
			"sessionDescriptions": descriptions,
			"config":              cfg,
			"generatedDigest":     d,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("POST /api/digest/send", func(w http.ResponseWriter, r *http.Request) {
		cfg := app.sessionStore.GetDigestConfig()
		if cfg.WebhookURL == "" {
			http.Error(w, "webhook URL not configured", http.StatusBadRequest)
			return
		}

		dateStr := r.URL.Query().Get("date")
		if dateStr == "" {
			dateStr = time.Now().In(resolveDigestLocation()).AddDate(0, 0, -1).Format("2006-01-02")
		}
		if _, err := time.Parse("2006-01-02", dateStr); err != nil {
			http.Error(w, "invalid date format, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}

		// SSE streaming progress
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		sendSSE := func(event string, data interface{}) {
			jsonData, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
			flusher.Flush()
		}

		type progressEvent struct {
			Step    string `json:"step"`
			Message string `json:"message"`
			Current int    `json:"current,omitempty"`
			Total   int    `json:"total,omitempty"`
		}

		result := generateDigestWithProgress(dateStr, func(step, message string, current, total int) {
			sendSSE("progress", progressEvent{
				Step:    step,
				Message: message,
				Current: current,
				Total:   total,
			})
		})

		sendSSE("progress", progressEvent{Step: "sending", Message: "發送到 Discord..."})

		if err := digestPkg.SendWebhook(http.DefaultClient, cfg.WebhookURL, result); err != nil {
			sendSSE("error", map[string]string{"message": fmt.Sprintf("發送失敗: %v", err)})
			return
		}

		sendSSE("done", map[string]string{"status": "sent", "date": dateStr})
	})
}
