package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/miskawu/piper/internal/claude"
	mousedexPkg "github.com/miskawu/piper/internal/mousedex"
	"github.com/miskawu/piper/internal/store"
	"github.com/miskawu/piper/internal/tmux"
	"github.com/miskawu/piper/internal/watcher"
)

func (app *App) registerSessionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		sessions, err := app.tmuxClient.ListSessions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := make([]sessionResponse, 0, len(sessions))
		for _, s := range sessions {
			content, _ := app.tmuxClient.CapturePane(s.Name)
			lastLine := tmux.GetLastSignificantLine(content)
			sr := sessionResponse{
				Session: watcher.Session{
					Name:     s.Name,
					Windows:  s.Windows,
					Created:  s.Created,
					Attached: s.Attached,
					Status:   "needs_input",
					LastLine: lastLine,
				},
			}
			if m := app.sessionStore.GetMeta(s.Name); m != nil {
				sr.Title = m.Title
				sr.Description = m.Description
				sr.DescriptionAuto = m.DescriptionAuto
				sr.Order = m.Order
				if m.ProjectPath != "" {
					sr.ProjectPath = m.ProjectPath
					sr.ProjectName = filepath.Base(m.ProjectPath)
				}
				if m.Title == "" || m.ProjectPath == "" {
					log.Printf("[session:list] session %q missing data: title=%q projectPath=%q", s.Name, m.Title, m.ProjectPath)
				}
			} else {
				log.Printf("[session:list] session %q has no meta in store", s.Name)
			}
			response = append(response, sr)
		}

		sort.Slice(response, func(i, j int) bool {
			return response[i].Order < response[j].Order
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	mux.HandleFunc("POST /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        string `json:"name"`
			WorkDir     string `json:"workDir"`
			ProjectPath string `json:"projectPath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		log.Printf("[session:create] request: name=%q workDir=%q projectPath=%q", req.Name, req.WorkDir, req.ProjectPath)

		if req.Name == "" {
			project := ""
			if req.ProjectPath != "" {
				project = tmux.SanitizeProjectName(filepath.Base(req.ProjectPath))
			} else if req.WorkDir != "" {
				project = tmux.SanitizeProjectName(filepath.Base(req.WorkDir))
			}
			if project != "" {
				req.Name = "ragent-" + project + "-" + tmux.GenerateShortID()
			} else {
				req.Name = "ragent-" + tmux.GenerateShortID()
				log.Printf("[session:create] WARNING: no projectPath or workDir, session %q will have no project info", req.Name)
			}
		}

		autoTitle := ""
		if req.ProjectPath != "" {
			autoTitle = strings.ToUpper(filepath.Base(req.ProjectPath))
		} else if req.WorkDir != "" {
			autoTitle = strings.ToUpper(filepath.Base(req.WorkDir))
		}

		var actualName string
		var err error
		if req.ProjectPath != "" {
			autoMode := false
			if stored := app.sessionStore.GetProjectAutoMode(req.ProjectPath); stored != nil {
				autoMode = *stored
			} else {
				autoMode = app.dcManager.DetectAutoMode(req.ProjectPath)
			}
			actualName, err = app.tmuxClient.CreateDevcontainerSession(req.Name, req.ProjectPath, autoMode)
		} else {
			actualName, err = app.tmuxClient.CreateSession(req.Name, req.WorkDir)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		group := tmux.ExtractGroupFromName(actualName)
		order := app.sessionStore.InsertAfterGroup(group, func(name string) string {
			return tmux.ExtractGroupFromName(name)
		})

		meta := app.sessionStore.GetMeta(actualName)
		if meta == nil {
			meta = &store.SessionMeta{}
		}
		if autoTitle != "" {
			meta.Title = autoTitle
		}
		meta.Order = order
		if req.ProjectPath != "" {
			meta.ProjectPath = req.ProjectPath
		} else if req.WorkDir != "" {
			meta.ProjectPath = req.WorkDir
		}
		log.Printf("[session:create] saved meta: name=%q title=%q projectPath=%q order=%d", actualName, meta.Title, meta.ProjectPath, meta.Order)
		if err := app.sessionStore.SetMeta(actualName, *meta); err != nil {
			log.Printf("[session:create] ERROR: failed to save meta for %s: %v", actualName, err)
		}

		// Immediately snapshot to recoverable so the session is protected from the start
		app.sessionStore.SnapshotRecoverable(app.sessionStore.AllSessions())

		if app.vaultManager != nil {
			cfg := mousedexPkg.GenerateMouseConfig(actualName)
			if isNew, err := app.vaultManager.CollectMouse(actualName, cfg); err != nil {
				log.Printf("WARNING: mousedex collect failed: %v", err)
			} else if isNew {
				log.Printf("mousedex: new species collected from %s", actualName)
			}
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "created", "name": actualName})
	})

	mux.HandleFunc("DELETE /api/sessions/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := app.tmuxClient.KillSession(name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcaster.onSessionClosed(name)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "killed"})
	})

	mux.HandleFunc("PUT /api/sessions/{name}/rename", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var req struct {
			NewName string `json:"newName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.NewName == "" {
			http.Error(w, "newName is required", http.StatusBadRequest)
			return
		}

		if err := app.tmuxClient.RenameSession(name, req.NewName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "renamed", "newName": req.NewName})
	})

	mux.HandleFunc("POST /api/sessions/{name}/send-keys", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var req struct {
			Keys string `json:"keys"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if err := app.tmuxClient.SendKeys(name, req.Keys); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
	})

	mux.HandleFunc("PUT /api/sessions/{name}/meta", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var req struct {
			Title       *string `json:"title"`
			Description *string `json:"description"`
			ProjectPath *string `json:"projectPath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		meta := app.sessionStore.GetMeta(name)
		if meta == nil {
			meta = &store.SessionMeta{}
		}
		if req.Title != nil {
			meta.Title = *req.Title
		}
		if req.Description != nil {
			meta.Description = *req.Description
			meta.DescriptionAuto = false
		}
		if req.ProjectPath != nil {
			meta.ProjectPath = *req.ProjectPath
		}
		if err := app.sessionStore.SetMeta(name, *meta); err != nil {
			http.Error(w, fmt.Sprintf("failed to save session meta: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(meta)
	})

	// --- Recovery endpoints ---

	mux.HandleFunc("GET /api/sessions/recoverable", func(w http.ResponseWriter, r *http.Request) {
		entries, err := app.sessionStore.ListRecoverable()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list recoverable: %v", err), http.StatusInternalServerError)
			return
		}

		// Exclude sessions that are currently active in tmux
		activeSessions, _ := app.tmuxClient.ListSessions()
		activeNames := make(map[string]bool, len(activeSessions))
		for _, s := range activeSessions {
			activeNames[s.Name] = true
		}

		type recoverableResponse struct {
			store.RecoverableSession
			IsDevContainer bool `json:"isDevContainer"`
			Active         bool `json:"active"`
		}

		var result []recoverableResponse
		for _, e := range entries {
			resp := recoverableResponse{
				RecoverableSession: e,
				Active:             activeNames[e.Name],
			}
			if e.ProjectPath != "" {
				if _, err := os.Stat(filepath.Join(e.ProjectPath, ".devcontainer", "devcontainer.json")); err == nil {
					resp.IsDevContainer = true
				}
			}
			result = append(result, resp)
		}
		if result == nil {
			result = []recoverableResponse{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("POST /api/sessions/{name}/restore", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")

		// Find in recoverable list
		entries, err := app.sessionStore.ListRecoverable()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list recoverable: %v", err), http.StatusInternalServerError)
			return
		}
		var found *store.RecoverableSession
		for _, e := range entries {
			if e.Name == name {
				found = &e
				break
			}
		}
		if found == nil {
			http.Error(w, "session not found in recoverable list", http.StatusNotFound)
			return
		}

		// Check not already in tmux
		if exists, _ := app.tmuxClient.HasSession(name); exists {
			http.Error(w, "session already exists in tmux", http.StatusConflict)
			return
		}

		// Determine session type
		isDevContainer := false
		if found.ProjectPath != "" {
			if _, err := os.Stat(filepath.Join(found.ProjectPath, ".devcontainer", "devcontainer.json")); err == nil {
				isDevContainer = true
			}
		}

		// Check if session JSONL exists (DevContainer uses per-project isolated dir)
		hasJSONL := false
		if found.ClaudeSessionID != "" {
			if isDevContainer {
				hasJSONL = claude.SessionJSONLPathForDevcontainer(found.ClaudeSessionID, found.ProjectPath) != ""
			} else {
				hasJSONL = claude.SessionJSONLPath(found.ClaudeSessionID, found.ProjectPath) != ""
			}
		}
		log.Printf("restore %s: claudeSessionID=%q hasJSONL=%v isDevContainer=%v", name, found.ClaudeSessionID, hasJSONL, isDevContainer)

		// Build claude command
		claudeCmd := "claude"
		if hasJSONL {
			claudeCmd = "claude --resume " + found.ClaudeSessionID
		}

		var actualName string
		if isDevContainer {
			autoMode := false
			if stored := app.sessionStore.GetProjectAutoMode(found.ProjectPath); stored != nil {
				autoMode = *stored
			} else if app.dcManager != nil {
				autoMode = app.dcManager.DetectAutoMode(found.ProjectPath)
			}
			var opts []tmux.DCOption
			if hasJSONL {
				opts = append(opts, tmux.WithResumeID(found.ClaudeSessionID))
			}
			actualName, err = app.tmuxClient.CreateDevcontainerSession(name, found.ProjectPath, autoMode, opts...)
		} else {
			actualName, err = app.tmuxClient.CreateSessionWithCommand(name, found.ProjectPath, claudeCmd)
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to restore session: %v", err), http.StatusInternalServerError)
			return
		}

		// Restore metadata to sessions.json
		if err := app.sessionStore.SetMeta(actualName, store.SessionMeta{
			Title:           found.Title,
			Description:     found.Description,
			ProjectPath:     found.ProjectPath,
			ClaudeSessionID: found.ClaudeSessionID,
		}); err != nil {
			log.Printf("WARNING: failed to restore session meta for %s: %v", actualName, err)
		}

		// Remove from recoverable
		if err := app.sessionStore.RemoveRecoverable(name); err != nil {
			log.Printf("WARNING: failed to remove recoverable entry %s: %v", name, err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "restored", "name": actualName})
	})

	mux.HandleFunc("POST /api/sessions/{name}/dismiss", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := app.sessionStore.RemoveRecoverable(name); err != nil {
			http.Error(w, fmt.Sprintf("failed to dismiss: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "dismissed"})
	})

	mux.HandleFunc("PUT /api/sessions/order", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Order []string `json:"order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := app.sessionStore.SetOrder(req.Order); err != nil {
			http.Error(w, fmt.Sprintf("failed to save order: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
}
