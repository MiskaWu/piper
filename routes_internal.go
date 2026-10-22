package main

import (
	"encoding/json"
	"net/http"
)

func (app *App) registerInternalRoutes(mux *http.ServeMux) {
	// POST /api/internal/hook-status — receives hook-based session status from DevContainer
	mux.HandleFunc("POST /api/internal/hook-status", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			SessionName string `json:"sessionName"`
			Status      string `json:"status"`
			Timestamp   int64  `json:"timestamp"`
			HookEvent   string `json:"hookEvent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if payload.SessionName == "" || payload.Status == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		app.hookData.SetHookStatus(payload.SessionName, payload.Status, payload.HookEvent, payload.Timestamp)

		if app.watcherTrigger != nil {
			app.watcherTrigger.Trigger()
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// POST /api/internal/session-meta — receives statusline metadata from DevContainer
	mux.HandleFunc("POST /api/internal/session-meta", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			SessionName    string `json:"sessionName"`
			Timestamp      int64  `json:"timestamp"`
			SessionID      string `json:"sessionId"`
			Model          string `json:"model"`
			CostUSD        float64 `json:"costUsd"`
			ContextUsedPct float64 `json:"contextUsedPct"`
			DurationMs     int64  `json:"durationMs"`
			RateLimits     *struct {
				FiveHour *struct {
					UsedPercentage float64 `json:"used_percentage"`
					ResetsAt       float64 `json:"resets_at"`
				} `json:"five_hour,omitempty"`
				SevenDay *struct {
					UsedPercentage float64 `json:"used_percentage"`
					ResetsAt       float64 `json:"resets_at"`
				} `json:"seven_day,omitempty"`
			} `json:"rateLimits,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if payload.SessionName == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Build meta entry
		meta := &hookMetaEntry{
			Timestamp:      payload.Timestamp,
			SessionID:      payload.SessionID,
			Model:          payload.Model,
			CostUSD:        payload.CostUSD,
			ContextUsedPct: payload.ContextUsedPct,
			DurationMs:     payload.DurationMs,
		}
		if payload.RateLimits != nil {
			rl := &hookRateLimits{}
			if payload.RateLimits.FiveHour != nil {
				rl.FiveHour = &hookRateLimitTier{
					UsedPercentage: payload.RateLimits.FiveHour.UsedPercentage,
					ResetsAt:       payload.RateLimits.FiveHour.ResetsAt,
				}
			}
			if payload.RateLimits.SevenDay != nil {
				rl.SevenDay = &hookRateLimitTier{
					UsedPercentage: payload.RateLimits.SevenDay.UsedPercentage,
					ResetsAt:       payload.RateLimits.SevenDay.ResetsAt,
				}
			}
			meta.RateLimits = rl
		}
		app.hookData.SetSessionMeta(payload.SessionName, meta)

		// Set ClaudeSessionID if reported and not yet known
		if payload.SessionID != "" {
			if m := app.sessionStore.GetMeta(payload.SessionName); m != nil && m.ClaudeSessionID == "" {
				app.sessionStore.SetClaudeSessionID(payload.SessionName, payload.SessionID)
			}
		}

		// Log rate limits snapshot for dashboard history
		if u := app.hookData.GetLatestUsage(); u != nil && app.usageLogger != nil {
			app.usageLogger.LogIfDue(u)
		}

		if app.watcherTrigger != nil {
			app.watcherTrigger.Trigger()
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
