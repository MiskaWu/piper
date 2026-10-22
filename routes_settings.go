package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/miskawu/piper/internal/plan"
	"github.com/miskawu/piper/internal/store"
)

func (app *App) registerSettingsRoutes(mux *http.ServeMux) {
	// Clipboard
	mux.HandleFunc("POST /api/clipboard/image", func(w http.ResponseWriter, r *http.Request) {
		if app.clipStore == nil {
			http.Error(w, "clipboard not configured", http.StatusInternalServerError)
			return
		}
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			http.Error(w, fmt.Sprintf("failed to parse form: %v", err), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			http.Error(w, "image field is required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		id, fullPath, saveErr := app.clipStore.Save(file, contentType)
		if saveErr != nil {
			if strings.Contains(saveErr.Error(), "unsupported content type") || strings.Contains(saveErr.Error(), "size exceeds") {
				http.Error(w, saveErr.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, fmt.Sprintf("failed to save image: %v", saveErr), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": id, "path": fullPath})
	})

	mux.HandleFunc("GET /api/clipboard/image/{id}", func(w http.ResponseWriter, r *http.Request) {
		if app.clipStore == nil {
			http.Error(w, "clipboard not configured", http.StatusInternalServerError)
			return
		}
		id := r.PathValue("id")
		path, lookupErr := app.clipStore.Path(id)
		if lookupErr != nil {
			http.Error(w, "image not found", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, path)
	})

	// Clipboard file (any type)
	mux.HandleFunc("POST /api/clipboard/file", func(w http.ResponseWriter, r *http.Request) {
		if app.clipStore == nil {
			http.Error(w, "clipboard not configured", http.StatusInternalServerError)
			return
		}
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			http.Error(w, fmt.Sprintf("failed to parse form: %v", err), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file field is required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		filename := header.Filename
		if filename == "" {
			http.Error(w, "filename is required", http.StatusBadRequest)
			return
		}

		id, fullPath, saveErr := app.clipStore.SaveFile(file, filename)
		if saveErr != nil {
			if strings.Contains(saveErr.Error(), "no extension") || strings.Contains(saveErr.Error(), "size exceeds") {
				http.Error(w, saveErr.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, fmt.Sprintf("failed to save file: %v", saveErr), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": id, "path": fullPath})
	})

	mux.HandleFunc("GET /api/clipboard/file/{id}", func(w http.ResponseWriter, r *http.Request) {
		if app.clipStore == nil {
			http.Error(w, "clipboard not configured", http.StatusInternalServerError)
			return
		}
		id := r.PathValue("id")
		path, lookupErr := app.clipStore.Path(id)
		if lookupErr != nil {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, path)
	})

	// Model Config
	validModels := map[string]bool{"": true, "haiku": true, "sonnet": true, "opus": true}

	mux.HandleFunc("GET /api/settings/models", func(w http.ResponseWriter, r *http.Request) {
		cfg := app.sessionStore.GetModelConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	})

	mux.HandleFunc("PUT /api/settings/models", func(w http.ResponseWriter, r *http.Request) {
		var cfg store.ModelConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		for _, m := range []string{cfg.DevcontainerGen, cfg.Summarizer, cfg.IdeaEnrich} {
			if !validModels[m] {
				http.Error(w, fmt.Sprintf("invalid model: %q (must be haiku, sonnet, opus, or empty)", m), http.StatusBadRequest)
				return
			}
		}
		if err := app.sessionStore.SetModelConfig(cfg); err != nil {
			http.Error(w, fmt.Sprintf("failed to save model config: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	})

	// Epic Git
	mux.HandleFunc("GET /api/settings/epic-git", func(w http.ResponseWriter, r *http.Request) {
		cfg := app.sessionStore.GetEpicGitConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	})

	mux.HandleFunc("PUT /api/settings/epic-git", func(w http.ResponseWriter, r *http.Request) {
		var cfg store.EpicGitConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if cfg.SyncIntervalMin < 0 {
			http.Error(w, "syncIntervalMin must be >= 0", http.StatusBadRequest)
			return
		}
		if cfg.RemoteURL != "" && app.vaultManager != nil {
			if err := app.vaultManager.Git().LsRemote(app.vaultManager.BaseDir(), cfg.RemoteURL); err != nil {
				http.Error(w, fmt.Sprintf("remote verification failed: %v — please check your SSH keys or git credentials", err), http.StatusBadRequest)
				return
			}
		}
		if cfg.RemoteURL != "" && app.vaultManager != nil {
			git := app.vaultManager.Git()
			if git.RemoteExists(app.vaultManager.BaseDir(), "origin") {
				if err := git.RemoteSetURL(app.vaultManager.BaseDir(), "origin", cfg.RemoteURL); err != nil {
					http.Error(w, fmt.Sprintf("failed to set remote URL: %v", err), http.StatusInternalServerError)
					return
				}
			} else {
				if err := git.RemoteAdd(app.vaultManager.BaseDir(), "origin", cfg.RemoteURL); err != nil {
					http.Error(w, fmt.Sprintf("failed to add remote: %v", err), http.StatusInternalServerError)
					return
				}
			}
		}
		if err := app.sessionStore.SetEpicGitConfig(cfg); err != nil {
			http.Error(w, fmt.Sprintf("failed to save epic git config: %v", err), http.StatusInternalServerError)
			return
		}
		if app.epicSyncer != nil {
			app.epicSyncer.Configure(cfg.SyncEnabled, cfg.SyncIntervalMin)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	})

	mux.HandleFunc("POST /api/settings/epic-git/sync", func(w http.ResponseWriter, r *http.Request) {
		if app.epicSyncer == nil {
			http.Error(w, "syncer not available", http.StatusServiceUnavailable)
			return
		}
		app.epicSyncer.TriggerSync()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "triggered"})
	})

	mux.HandleFunc("GET /api/settings/epic-git/status", func(w http.ResponseWriter, r *http.Request) {
		if app.epicSyncer == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(plan.SyncStatus{})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(app.epicSyncer.Status())
	})

	// Firewall config
	mux.HandleFunc("GET /api/settings/firewall", func(w http.ResponseWriter, r *http.Request) {
		cfg := app.sessionStore.GetFirewallConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	})

	mux.HandleFunc("PUT /api/settings/firewall", func(w http.ResponseWriter, r *http.Request) {
		var cfg store.FirewallConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		switch cfg.Mode {
		case "open", "whitelist":
		default:
			http.Error(w, "invalid mode: must be open or whitelist", http.StatusBadRequest)
			return
		}
		seen := make(map[string]bool)
		cleaned := make([]string, 0, len(cfg.Domains))
		for _, d := range cfg.Domains {
			d = strings.TrimSpace(d)
			if d == "" || seen[d] {
				continue
			}
			seen[d] = true
			cleaned = append(cleaned, d)
		}
		cfg.Domains = cleaned

		if err := app.sessionStore.SetFirewallConfig(cfg); err != nil {
			http.Error(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	})

	// Digest config
	mux.HandleFunc("GET /api/settings/digest", func(w http.ResponseWriter, r *http.Request) {
		cfg := app.sessionStore.GetDigestConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	})

	mux.HandleFunc("PUT /api/settings/digest", func(w http.ResponseWriter, r *http.Request) {
		var cfg store.DigestConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if cfg.SendHour < 0 || cfg.SendHour > 23 {
			http.Error(w, "sendHour must be 0-23", http.StatusBadRequest)
			return
		}
		if err := app.sessionStore.SetDigestConfig(cfg); err != nil {
			http.Error(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	})

	// Recovery config
	mux.HandleFunc("GET /api/settings/recovery", func(w http.ResponseWriter, r *http.Request) {
		cfg := app.sessionStore.GetRecoveryConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	})

	mux.HandleFunc("PUT /api/settings/recovery", func(w http.ResponseWriter, r *http.Request) {
		var cfg store.RecoveryConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		validDays := map[int]bool{3: true, 7: true, 14: true, 30: true}
		if !validDays[cfg.RetentionDays] {
			http.Error(w, "retentionDays must be one of: 3, 7, 14, 30", http.StatusBadRequest)
			return
		}
		if err := app.sessionStore.SetRecoveryConfig(cfg); err != nil {
			http.Error(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	})
}
