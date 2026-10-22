package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/miskawu/piper/internal/browse"
	"github.com/miskawu/piper/internal/devcontainer"
	mousedexPkg "github.com/miskawu/piper/internal/mousedex"
	"github.com/miskawu/piper/internal/plan"
	"github.com/miskawu/piper/internal/vault"
)

// pinLatestClaudeVersion writes the cached latest Claude Code version into the
// project's devcontainer.json so Docker cache is invalidated on rebuild.
func pinLatestClaudeVersion(ctx context.Context, projectPath string) {
	if v := devcontainer.GetLatestClaudeVersionCached(ctx); v != "" {
		if err := devcontainer.PinClaudeVersion(projectPath, v); err != nil {
			log.Printf("Warning: pin Claude version: %v", err)
		}
	}
}

// resolveProjectPath finds a project path by slug (preferred) or basename (fallback).
func (app *App) resolveProjectPath(name string) (string, bool) {
	if entry, ok := app.sessionStore.ResolveBySlug(name); ok {
		return entry.Path, true
	}
	if entry, ok := app.sessionStore.ResolveByBasename(name); ok {
		return entry.Path, true
	}
	return "", false
}

func (app *App) registerProjectRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		paths := app.sessionStore.GetProjects()
		projects, err := app.dcManager.GetProjectStates(paths)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get project states: %v", err), http.StatusInternalServerError)
			return
		}

		type projectWithMeta struct {
			devcontainer.Project
			Slug                 string `json:"slug"`
			AutoMode             *bool  `json:"autoMode"`
			UpdateAvailable      bool   `json:"updateAvailable"`
			ClaudeUpdateAvailable bool  `json:"claudeUpdateAvailable"`
			HasManifest          bool   `json:"hasManifest"`
		}

		// Build path→slug map from store entries
		slugMap := make(map[string]string)
		for _, entry := range app.sessionStore.GetProjectEntries() {
			slugMap[entry.Path] = entry.Slug
		}

		// Check latest Claude version once (cached, 1h TTL)
		latestClaude := devcontainer.GetLatestClaudeVersionCached(r.Context())

		enriched := make([]projectWithMeta, len(projects))
		for i, p := range projects {
			enriched[i].Project = p
			enriched[i].Slug = slugMap[p.Path]
			if stored := app.sessionStore.GetProjectAutoMode(p.Path); stored != nil {
				enriched[i].AutoMode = stored
			} else {
				detected := app.dcManager.DetectAutoMode(p.Path)
				enriched[i].AutoMode = &detected
			}
			enriched[i].UpdateAvailable = devcontainer.GetProjectVersions(p.Path).UpdateAvailable()
			if latestClaude != "" && p.ContainerState == "running" && p.ContainerID != "" {
				if current, err := app.dcManager.GetClaudeVersion(p.ContainerID); err == nil {
					enriched[i].ClaudeUpdateAvailable = current != latestClaude
				}
			}
			if _, err := os.Stat(plan.ManifestPath(p.Path)); err == nil {
				enriched[i].HasManifest = true
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"projects":        enriched,
			"devcontainerCLI": app.dcManager.CLIAvailable(),
		})
	})

	mux.HandleFunc("PUT /api/projects/order", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Order []string `json:"order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := app.sessionStore.SetProjectOrder(req.Order); err != nil {
			http.Error(w, fmt.Sprintf("failed to save project order: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Path == "" {
			http.Error(w, "PATH_REQUIRED", http.StatusBadRequest)
			return
		}
		if !filepath.IsAbs(req.Path) {
			http.Error(w, "PATH_NOT_ABSOLUTE", http.StatusBadRequest)
			return
		}

		dcPath := filepath.Join(req.Path, ".devcontainer")
		if _, err := os.Stat(dcPath); os.IsNotExist(err) {
			http.Error(w, "DEVCONTAINER_NOT_FOUND", http.StatusBadRequest)
			return
		}

		if err := app.sessionStore.AddProject(req.Path); err != nil {
			http.Error(w, fmt.Sprintf("failed to add project: %v", err), http.StatusInternalServerError)
			return
		}

		projects, err := app.dcManager.GetProjectStates([]string{req.Path})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get project state: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(projects[0])
	})

	mux.HandleFunc("DELETE /api/projects/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		targetPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		if err := app.sessionStore.RemoveProject(targetPath); err != nil {
			http.Error(w, fmt.Sprintf("failed to remove project: %v", err), http.StatusInternalServerError)
			return
		}

		cleanupFlags := r.URL.Query()["cleanup"]
		cleanupSet := make(map[string]bool)
		for _, f := range cleanupFlags {
			cleanupSet[f] = true
		}

		result := map[string]interface{}{"status": "removed"}

		if cleanupSet["containers"] || cleanupSet["images"] {
			if cleanupSet["containers"] && cleanupSet["images"] {
				cr := app.dcManager.Cleanup(targetPath)
				result["containerRemoved"] = cr.ContainerErr == nil
				result["imageRemoved"] = cr.ImageErr == nil
				if cr.ContainerErr != nil {
					result["containerError"] = cr.ContainerErr.Error()
				}
				if cr.ImageErr != nil {
					result["imageError"] = cr.ImageErr.Error()
				}
			} else if cleanupSet["containers"] {
				err := app.dcManager.RemoveContainer(targetPath)
				result["containerRemoved"] = err == nil
				if err != nil {
					result["containerError"] = err.Error()
				}
			} else if cleanupSet["images"] {
				err := app.dcManager.RemoveImage(targetPath)
				result["imageRemoved"] = err == nil
				if err != nil {
					result["imageError"] = err.Error()
				}
			}
		}

		// Clean up per-project DevContainer session data
		encoded := devcontainer.EncodeProjectPath(targetPath)
		if encoded != "" && encoded != "-" {
			home, _ := os.UserHomeDir()
			dcDataDir := filepath.Join(home, ".claude-devcontainers", encoded)
			if err := os.RemoveAll(dcDataDir); err != nil {
				log.Printf("WARNING: failed to clean devcontainer data %s: %v", dcDataDir, err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("PUT /api/projects/{name}/auto-mode", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		targetPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		var req struct {
			AutoMode bool `json:"autoMode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := app.sessionStore.SetProjectAutoMode(targetPath, &req.AutoMode); err != nil {
			http.Error(w, fmt.Sprintf("failed to save auto mode: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "autoMode": req.AutoMode})
	})

	mux.HandleFunc("GET /api/projects/{name}/manifest", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		projectPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		data, err := plan.ReadManifest(projectPath)
		if err != nil {
			if errors.Is(err, plan.ErrManifestNotFound) {
				http.Error(w, "manifest not found (run /manifest in project to generate)", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to read manifest: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Write(data)
	})

	// Bindings live under /api/bindings/* instead of /api/projects/{name}/bindings/*
	// because Go ServeMux can't disambiguate {name}="setup" vs literal "setup"
	// in the /api/projects/setup/{id} namespace.
	mux.HandleFunc("GET /api/bindings/{project}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("project")
		projectPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		ids, err := plan.ListBindings(projectPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list bindings: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"contractIds": ids})
	})

	mux.HandleFunc("GET /api/bindings/{project}/{id}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("project")
		id := r.PathValue("id")
		projectPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		b, err := plan.ReadBinding(projectPath, id)
		if err != nil {
			if errors.Is(err, plan.ErrBindingNotFound) {
				http.Error(w, "binding not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, plan.ErrInvalidContractID) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, fmt.Sprintf("failed to read binding: %v", err), http.StatusInternalServerError)
			return
		}
		data, err := plan.MarshalBinding(b)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to marshal binding: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Write(data)
	})

	mux.HandleFunc("PUT /api/bindings/{project}/{id}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("project")
		id := r.PathValue("id")
		projectPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read body: %v", err), http.StatusBadRequest)
			return
		}
		b, err := plan.ParseBinding(body)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid binding yaml: %v", err), http.StatusBadRequest)
			return
		}
		if b.ContractID != "" && b.ContractID != id {
			http.Error(w, fmt.Sprintf("contract_id in body (%q) does not match URL id (%q)", b.ContractID, id), http.StatusBadRequest)
			return
		}
		b.ContractID = id
		if err := plan.WriteBinding(projectPath, b); err != nil {
			if errors.Is(err, plan.ErrInvalidContractID) || errors.Is(err, plan.ErrBindingInvalid) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, fmt.Sprintf("failed to write binding: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "saved", "contractId": id})
	})

	mux.HandleFunc("DELETE /api/bindings/{project}/{id}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("project")
		id := r.PathValue("id")
		projectPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		if err := plan.DeleteBinding(projectPath, id); err != nil {
			if errors.Is(err, plan.ErrBindingNotFound) {
				http.Error(w, "binding not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, plan.ErrInvalidContractID) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, fmt.Sprintf("failed to delete binding: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "contractId": id})
	})

	mux.HandleFunc("POST /api/projects/{name}/rebuild", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		projectPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		pinLatestClaudeVersion(r.Context(), projectPath)
		if err := app.dcManager.Rebuild(projectPath); err != nil {
			http.Error(w, fmt.Sprintf("rebuild failed: %v", err), http.StatusInternalServerError)
			return
		}
		app.sessionStore.SetProjectAutoMode(projectPath, nil)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "rebuilt"})
	})

	mux.HandleFunc("POST /api/projects/{name}/upgrade", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		projectPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}

		versions := devcontainer.GetProjectVersions(projectPath)
		if !versions.UpdateAvailable() {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "up_to_date"})
			return
		}

		fwCfg := app.sessionStore.GetFirewallConfig()
		upgradeCfg := devcontainer.UpgradeConfig{
			ProjectPath:    projectPath,
			ProjectName:    name,
			FirewallMode:   fwCfg.Mode,
			AllowedDomains: strings.Join(fwCfg.Domains, ","),
		}

		result, err := app.upgrader.UpgradeFiles(r.Context(), upgradeCfg)
		if err != nil {
			http.Error(w, fmt.Sprintf("upgrade failed: %v", err), http.StatusInternalServerError)
			return
		}

		pinLatestClaudeVersion(r.Context(), projectPath)
		// Rebuild container to apply changes
		if rebuildErr := app.dcManager.Rebuild(projectPath); rebuildErr != nil {
			http.Error(w, fmt.Sprintf("upgrade succeeded but rebuild failed: %v", rebuildErr), http.StatusInternalServerError)
			return
		}
		app.sessionStore.SetProjectAutoMode(projectPath, nil)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("POST /api/projects/{name}/update-claude", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		projectPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}

		projects, err := app.dcManager.GetProjectStates([]string{projectPath})
		if err != nil || len(projects) == 0 || projects[0].ContainerID == "" {
			http.Error(w, "container not found", http.StatusNotFound)
			return
		}
		if projects[0].ContainerState != "running" {
			http.Error(w, "container not running", http.StatusConflict)
			return
		}
		containerID := projects[0].ContainerID

		current, err := app.dcManager.GetClaudeVersion(containerID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get current version: %v", err), http.StatusInternalServerError)
			return
		}

		latest, err := devcontainer.FetchLatestClaudeVersion(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to fetch latest version: %v", err), http.StatusInternalServerError)
			return
		}

		if current == latest {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "up_to_date",
				"current": current,
				"latest":  latest,
			})
			return
		}

		updated, err := app.dcManager.UpdateClaude(containerID)
		if err != nil {
			http.Error(w, fmt.Sprintf("update failed: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "updated",
			"previous": current,
			"current":  updated,
			"latest":   latest,
		})
	})

	mux.HandleFunc("GET /api/projects/{name}/logs", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		targetPath, ok := app.resolveProjectPath(name)
		if !ok {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		projects, err := app.dcManager.GetProjectStates([]string{targetPath})
		if err != nil || len(projects) == 0 || projects[0].ContainerID == "" {
			http.Error(w, "project or container not found", http.StatusNotFound)
			return
		}
		logs, err := app.dcManager.GetContainerLogs(projects[0].ContainerID, 50)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get logs: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"logs": logs})
	})

	mux.HandleFunc("POST /api/projects/setup", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name     string `json:"name"`
			Path     string `json:"path"`
			AutoMode *bool  `json:"autoMode,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if req.Path == "" || !filepath.IsAbs(req.Path) {
			http.Error(w, "path must be an absolute path", http.StatusBadRequest)
			return
		}

		setupID := generateSetupID()
		ctx, cancel := context.WithCancel(context.Background())

		as := &activeSetup{
			ID:     setupID,
			Status: devcontainer.SetupStatus{Phase: devcontainer.PhaseWriting, Message: "正在啟動..."},
			Cancel: cancel,
			Done:   make(chan struct{}),
		}
		app.setupTracker.set(setupID, as)

		go func() {
			defer close(as.Done)
			progress := func(status devcontainer.SetupStatus) {
				app.setupTracker.mu.Lock()
				as.Status = status
				app.setupTracker.mu.Unlock()

				msg, _ := json.Marshal(map[string]interface{}{
					"type":    "setup_progress",
					"setupID": setupID,
					"phase":   status.Phase,
					"message": status.Message,
					"error":   status.Error,
				})
				app.hub.Broadcast(msg)
			}

			fwCfg := app.sessionStore.GetFirewallConfig()
			fwDomains := strings.Join(fwCfg.Domains, ",")

			actualPath, err := app.setupRunner.Run(ctx, devcontainer.SetupConfig{
				ProjectName:    req.Name,
				ProjectPath:    req.Path,
				FirewallMode:   fwCfg.Mode,
				AllowedDomains: fwDomains,
			}, progress)

			if err == nil {
				if addErr := app.sessionStore.AddProject(actualPath); addErr != nil {
					log.Printf("WARNING: failed to register project %s: %v", actualPath, addErr)
				}
				if req.AutoMode != nil {
					if amErr := app.sessionStore.SetProjectAutoMode(actualPath, req.AutoMode); amErr != nil {
						log.Printf("WARNING: failed to set autoMode for %s: %v", actualPath, amErr)
					}
				}
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"setupID": setupID})
	})

	mux.HandleFunc("GET /api/projects/setup/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		as := app.setupTracker.get(id)
		if as == nil {
			http.Error(w, "setup not found", http.StatusNotFound)
			return
		}
		app.setupTracker.mu.RLock()
		status := as.Status
		app.setupTracker.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("DELETE /api/projects/setup/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		as := app.setupTracker.get(id)
		if as == nil {
			http.Error(w, "setup not found", http.StatusNotFound)
			return
		}
		as.Cancel()
		app.setupTracker.delete(id)
		w.WriteHeader(http.StatusNoContent)
	})

	// Browse directories
	mux.HandleFunc("GET /api/directories/browse", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			path = os.Getenv("HOME")
		}
		result, err := browse.ListDir(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// Browse all files (files + directories)
	mux.HandleFunc("GET /api/directories/files", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "path query parameter required", http.StatusBadRequest)
			return
		}
		result, err := browse.ListAll(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// Mousedex
	mux.HandleFunc("GET /api/mousedex", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		mdex, err := app.vaultManager.LoadMousedex()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp := struct {
			Collected map[string]vault.MousedexEntry `json:"collected"`
			Total     int                            `json:"total"`
			Count     int                            `json:"count"`
		}{
			Collected: mdex.Collected,
			Total:     mousedexPkg.TotalSpecies,
			Count:     len(mdex.Collected),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("GET /api/mousedex/stats", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		stats, err := app.vaultManager.GetMousedexStats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})
}
