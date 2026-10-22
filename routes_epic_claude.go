package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/miskawu/piper/internal/devcontainer"
	"github.com/miskawu/piper/internal/epiccontainer"
)

func (app *App) registerEpicClaudeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/epics/claude-version", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"dockerAvailable": app.dockerAvailable,
		}

		if !app.dockerAvailable {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		var (
			wg                       sync.WaitGroup
			current, latest          string
			currentErr               error
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			current, currentErr = epiccontainer.GetImageClaudeVersion(r.Context())
		}()
		go func() {
			defer wg.Done()
			latest = devcontainer.GetLatestClaudeVersionCached(r.Context())
		}()
		wg.Wait()

		if currentErr != nil {
			resp["error"] = currentErr.Error()
		}
		resp["current"] = current
		resp["imageExists"] = current != ""
		if latest != "" {
			resp["latest"] = latest
			resp["needsUpdate"] = current != "" && current != latest
		}

		if app.epicUpdater != nil {
			resp["update"] = app.epicUpdater.Snapshot()
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /api/epics/claude-update", func(w http.ResponseWriter, r *http.Request) {
		if !app.dockerAvailable {
			http.Error(w, "docker not available", http.StatusBadRequest)
			return
		}
		if app.epicUpdater == nil {
			http.Error(w, "epic updater not initialized", http.StatusInternalServerError)
			return
		}

		latest := devcontainer.GetLatestClaudeVersionCached(r.Context())
		if latest == "" {
			http.Error(w, "failed to resolve latest claude version", http.StatusBadGateway)
			return
		}

		if err := app.epicUpdater.Start(r.Context(), devcontainer.TemplateFS(), latest); err != nil {
			if err == epiccontainer.ErrUpdateInProgress {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "started",
			"targetVersion": latest,
		})
	})
}
