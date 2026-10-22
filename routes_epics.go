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
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/miskawu/piper/internal/devcontainer"
	dispatchPkg "github.com/miskawu/piper/internal/dispatch"
	"github.com/miskawu/piper/internal/epiccontainer"
	"github.com/miskawu/piper/internal/plan"
	retroPkg "github.com/miskawu/piper/internal/retro"
	"github.com/miskawu/piper/internal/store"
)

// ensureEpicImage calls the injectable epicImageEnsurer or the default.
func (app *App) ensureEpicImage() error {
	if app.epicImageEnsurer != nil {
		return app.epicImageEnsurer()
	}
	return epiccontainer.EnsureImage(devcontainer.TemplateFS())
}

// createEpicContainerSession handles the shared boilerplate for plan/dispatch/retro session creation.
// When Docker is available, runs Claude in an isolated container; otherwise falls back to a native
// tmux session (without --dangerously-skip-permissions).
func (app *App) createEpicContainerSession(sessionName, epicSlug string, sessionType epiccontainer.SessionType) (string, error) {
	if app.dockerAvailable {
		if app.epicUpdater != nil && app.epicUpdater.InProgress() {
			return "", fmt.Errorf("epic image update in progress — please wait")
		}
		if err := app.ensureEpicImage(); err != nil {
			return "", fmt.Errorf("ensure epic image: %w", err)
		}
		workDir, err := epiccontainer.PrepareWorkspace(sessionName, epicSlug, sessionType)
		if err != nil {
			return "", fmt.Errorf("prepare workspace: %w", err)
		}
		cfg := epiccontainer.RunConfig{
			SessionName: sessionName,
			ClaudeHome:  filepath.Join(os.Getenv("HOME"), ".claude"),
			WorkDir:     workDir,
		}
		script := epiccontainer.BuildTmuxScript(epiccontainer.BuildDockerArgs(cfg))
		name, err := app.tmuxClient.CreateEpicContainerSession(sessionName, script)
		if err != nil {
			epiccontainer.CleanWorkspace(sessionName)
			return "", err
		}
		return name, nil
	}

	// Native fallback: prepare workspace without isolation, run claude directly
	workDir, err := epiccontainer.PrepareNativeWorkspace(sessionName, epicSlug, sessionType)
	if err != nil {
		return "", fmt.Errorf("prepare native workspace: %w", err)
	}
	name, err := app.tmuxClient.CreateNativeEpicSession(sessionName, workDir)
	if err != nil {
		epiccontainer.CleanWorkspace(sessionName)
		return "", err
	}
	return name, nil
}

// cleanupEpicSession stops the epic session and removes the temp workspace.
// If Docker is available, also removes the Docker container.
func (app *App) cleanupEpicSession(sessionName string) {
	if app.dockerAvailable {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "docker", "rm", "-f", sessionName).Run(); err != nil {
			log.Printf("WARNING: docker rm %s: %v", sessionName, err)
		}
	}
	epiccontainer.CleanWorkspace(sessionName)
}

// enrichInfraStatus supplements InfraStatus with registered projects
// that are not already present from the story map scan.
func enrichInfraStatus(status *dispatchPkg.InfraStatus, entries []store.ProjectEntry) {
	existing := make(map[string]bool, len(status.Projects))
	for _, p := range status.Projects {
		existing[p.Path] = true
	}
	for _, entry := range entries {
		if existing[entry.Path] {
			continue
		}
		v := dispatchPkg.ReadInfraVersion(entry.Path)
		name := entry.Slug
		if name == "" {
			name = filepath.Base(entry.Path)
		}
		status.Projects = append(status.Projects, dispatchPkg.InfraProjectStatus{
			Name:            name,
			Path:            entry.Path,
			Version:         v,
			UpdateAvailable: v < plan.InfraVersion,
		})
	}
}

// verifyEpicExists checks if an epic exists by ID. Returns true if found.
func (app *App) verifyEpicExists(name string) (bool, error) {
	epics, err := app.vaultManager.ListEpics()
	if err != nil {
		return false, err
	}
	for _, e := range epics {
		if e.ID == name {
			return true, nil
		}
	}
	return false, nil
}

func (app *App) registerEpicRoutes(mux *http.ServeMux) {
	if app.vaultManager == nil {
		return
	}

	app.registerEpicClaudeRoutes(mux)

	mux.HandleFunc("GET /api/epics", func(w http.ResponseWriter, r *http.Request) {
		epics, err := app.vaultManager.ListEpics()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list epics: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(epics)
	})

	mux.HandleFunc("POST /api/epics", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		slug, err := app.vaultManager.CreateEpic(req.Name)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if strings.Contains(err.Error(), "empty") {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("create", "epic", slug)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": slug, "name": req.Name})
	})

	mux.HandleFunc("DELETE /api/epics/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := app.vaultManager.DeleteEpic(name); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "invalid epic id") {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = app.tmuxClient.KillSession("pplan-" + name)
		app.broadcastPlanUpdate("delete", "epic", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	})

	mux.HandleFunc("GET /api/epics/{name}/story-map", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		content, err := app.vaultManager.ReadStoryMap(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/yaml")
		w.Write([]byte(content))
	})

	mux.HandleFunc("GET /api/epics/{name}/story-map/stats", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		content, err := app.vaultManager.ReadStoryMap(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sm, err := plan.ParseStoryMap([]byte(content))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to parse story map: %v", err), http.StatusInternalServerError)
			return
		}
		stats := sm.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	mux.HandleFunc("PUT /api/epics/{name}/story-map", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		// Validate YAML structure + semantics before saving
		validated, validationErrs := plan.ValidateStoryMap(body)
		if validationErrs != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": validationErrs,
			})
			return
		}
		// New epics have no prior state — first write skips transition checks.
		if prevRaw, prevErr := app.vaultManager.ReadStoryMap(name); prevErr == nil {
			if prevSM, perr := plan.ParseStoryMap([]byte(prevRaw)); perr == nil {
				if transitionErrs := plan.ValidateStatusTransitions(prevSM, validated); len(transitionErrs) > 0 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"errors": transitionErrs,
					})
					return
				}
			}
		}
		// Re-marshal to ensure normalized output (e.g. nil tags → [])
		normalized, err := plan.MarshalStoryMap(validated)
		if err != nil {
			http.Error(w, "failed to normalize story map: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := app.vaultManager.WriteStoryMap(name, string(normalized)); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("update", "story-map", name)
		w.Header().Set("Content-Type", "application/json")
		warnings := validated.Warnings()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "saved",
			"warnings": warnings,
		})
	})

	mux.HandleFunc("PUT /api/epics/{name}/rename", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := app.vaultManager.RenameEpic(name, req.Name); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "empty") {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("rename", "epic", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": name, "name": req.Name})
	})

	// Plan session
	mux.HandleFunc("POST /api/epics/{name}/plan-session", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if found, err := app.verifyEpicExists(name); err != nil {
			http.Error(w, fmt.Sprintf("failed to list epics: %v", err), http.StatusInternalServerError)
			return
		} else if !found {
			http.Error(w, "epic not found", http.StatusNotFound)
			return
		}

		sessionName := "pplan-" + name
		alreadyExists, _ := app.tmuxClient.HasSession(sessionName)

		if !alreadyExists {
			var err error
			sessionName, err = app.createEpicContainerSession(sessionName, name, epiccontainer.SessionPlan)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to create plan session: %v", err), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if alreadyExists {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusCreated)
		}
		json.NewEncoder(w).Encode(map[string]string{"session": sessionName})
	})

	mux.HandleFunc("GET /api/epics/{name}/plan-session", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		sessionName := "pplan-" + name
		exists, _ := app.tmuxClient.HasSession(sessionName)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"session": sessionName,
			"exists":  exists,
		})
	})

	mux.HandleFunc("DELETE /api/epics/{name}/plan-session", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		sessionName := "pplan-" + name
		_ = app.tmuxClient.KillSession(sessionName)
		app.cleanupEpicSession(sessionName)
		if app.epicSyncer != nil {
			app.epicSyncer.TriggerSync()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "killed"})
	})

	// Dispatch session
	mux.HandleFunc("POST /api/epics/{name}/dispatch-session", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if found, err := app.verifyEpicExists(name); err != nil {
			http.Error(w, fmt.Sprintf("failed to list epics: %v", err), http.StatusInternalServerError)
			return
		} else if !found {
			http.Error(w, "epic not found", http.StatusNotFound)
			return
		}

		sessionName := "pdisp-" + name
		alreadyExists, _ := app.tmuxClient.HasSession(sessionName)

		if !alreadyExists {
			var err error
			sessionName, err = app.createEpicContainerSession(sessionName, name, epiccontainer.SessionDispatch)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to create dispatch session: %v", err), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if alreadyExists {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusCreated)
		}
		json.NewEncoder(w).Encode(map[string]string{"session": sessionName})
	})

	mux.HandleFunc("GET /api/epics/{name}/dispatch-session", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		sessionName := "pdisp-" + name
		exists, _ := app.tmuxClient.HasSession(sessionName)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"session": sessionName,
			"exists":  exists,
		})
	})

	mux.HandleFunc("DELETE /api/epics/{name}/dispatch-session", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		sessionName := "pdisp-" + name
		_ = app.tmuxClient.KillSession(sessionName)
		app.cleanupEpicSession(sessionName)
		if app.epicSyncer != nil {
			app.epicSyncer.TriggerSync()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "killed"})
	})

	// Retro session
	mux.HandleFunc("POST /api/epics/{name}/retro-session", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if found, err := app.verifyEpicExists(name); err != nil {
			http.Error(w, fmt.Sprintf("failed to list epics: %v", err), http.StatusInternalServerError)
			return
		} else if !found {
			http.Error(w, "epic not found", http.StatusNotFound)
			return
		}

		sessionName := "pretro-" + name
		alreadyExists, _ := app.tmuxClient.HasSession(sessionName)

		if !alreadyExists {
			var err error
			sessionName, err = app.createEpicContainerSession(sessionName, name, epiccontainer.SessionRetro)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to create retro session: %v", err), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if alreadyExists {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusCreated)
		}
		json.NewEncoder(w).Encode(map[string]string{"session": sessionName})
	})

	mux.HandleFunc("GET /api/epics/{name}/retro-session", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		sessionName := "pretro-" + name
		exists, _ := app.tmuxClient.HasSession(sessionName)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"session": sessionName,
			"exists":  exists,
		})
	})

	mux.HandleFunc("DELETE /api/epics/{name}/retro-session", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		sessionName := "pretro-" + name
		_ = app.tmuxClient.KillSession(sessionName)
		app.cleanupEpicSession(sessionName)
		if app.epicSyncer != nil {
			app.epicSyncer.TriggerSync()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "killed"})
	})

	// Dispatch/Retro automation
	projResolver := &storeProjectResolver{store: app.sessionStore}
	dispatchSvc := dispatchPkg.New(app.vaultManager, projResolver, app.vaultManager.EpicManager().BaseDir())
	retroSvc := retroPkg.New(app.vaultManager, projResolver).
		WithExecutor(app.vaultManager).
		WithContractLoader(app.vaultManager).
		WithDecisionLoader(app.vaultManager)

	mux.HandleFunc("POST /api/epics/{name}/dispatch/preview", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var body struct {
			ReleaseID string `json:"releaseId"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		result, err := dispatchSvc.Preview(name, body.ReleaseID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("POST /api/epics/{name}/dispatch/execute", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var req dispatchPkg.DispatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.EpicID = name

		result, err := dispatchSvc.Execute(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if app.epicSyncer != nil {
			app.epicSyncer.TriggerSync()
		}
		app.broadcastPlanUpdate("update", "story-map", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("GET /api/epics/{name}/dispatch/infra-status", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		result, err := dispatchSvc.GetInfraStatus(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		enrichInfraStatus(result, app.sessionStore.GetProjectEntries())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("POST /api/epics/{name}/dispatch/sync-infra", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var body struct {
			Projects []string `json:"projects"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		// Get infra status to find projects that need updating
		status, err := dispatchSvc.GetInfraStatus(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		enrichInfraStatus(status, app.sessionStore.GetProjectEntries())

		// Filter to requested projects, or all that need updating
		requested := make(map[string]bool)
		for _, p := range body.Projects {
			requested[p] = true
		}

		type syncResult struct {
			Name        string `json:"name"`
			FromVersion int    `json:"fromVersion"`
			ToVersion   int    `json:"toVersion"`
		}
		var results []syncResult

		for _, proj := range status.Projects {
			if len(requested) > 0 && !requested[proj.Name] {
				continue
			}
			if !proj.UpdateAvailable {
				continue
			}
			if err := dispatchSvc.SyncInfra(proj.Path); err != nil {
				http.Error(w, fmt.Sprintf("sync %s: %v", proj.Name, err), http.StatusInternalServerError)
				return
			}
			results = append(results, syncResult{
				Name:        proj.Name,
				FromVersion: proj.Version,
				ToVersion:   status.CurrentVersion,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"updated": results})
	})

	mux.HandleFunc("POST /api/epics/{name}/retro/preview", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		result, err := retroSvc.Preview(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("POST /api/epics/{name}/retro/execute", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var req retroPkg.ExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.EpicID = name

		result, err := retroSvc.Execute(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if app.epicSyncer != nil {
			app.epicSyncer.TriggerSync()
		}
		app.broadcastPlanUpdate("update", "story-map", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// --- Retro ---

	mux.HandleFunc("POST /api/epics/{name}/retro/generate", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var body struct {
			ReleaseLabel string `json:"releaseLabel"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if body.ReleaseLabel == "" {
			http.Error(w, "releaseLabel is required", http.StatusBadRequest)
			return
		}

		data, err := retroSvc.BuildReportData(name, body.ReleaseLabel)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		report, err := retroPkg.RenderReport(data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reportName := retroPkg.RetroReportName(body.ReleaseLabel)
		if snapshot, jerr := json.Marshal(data); jerr == nil {
			if werr := app.vaultManager.WriteRetroReportJSON(name, reportName, snapshot); werr != nil {
				log.Printf("warning: write retro snapshot %s/%s: %v", name, reportName, werr)
			}
		}
		if err := app.vaultManager.WriteRetroReport(name, reportName, report); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"name": reportName, "content": report})
	})

	mux.HandleFunc("GET /api/epics/{name}/retro/trends", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		trends, err := retroSvc.GetTrends(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trends)
	})

	mux.HandleFunc("GET /api/epics/{name}/retro/reports", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		reports, err := app.vaultManager.ListRetroReports(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if reports == nil {
			reports = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(reports)
	})

	mux.HandleFunc("GET /api/epics/{name}/retro/reports/{release}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		release := r.PathValue("release")
		if r.URL.Query().Get("format") == "json" {
			raw, err := app.vaultManager.ReadRetroReportJSON(name, release)
			if err != nil {
				status := http.StatusNotFound
				if !errors.Is(err, os.ErrNotExist) {
					status = http.StatusInternalServerError
				}
				http.Error(w, err.Error(), status)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(raw)
			return
		}
		content, err := app.vaultManager.ReadRetroReport(name, release)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
	})

	mux.HandleFunc("GET /api/epics/{name}/retro/actions", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if r.URL.Query().Get("format") == "json" {
			actions, err := retroSvc.ListActions(name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"actions": actions})
			return
		}
		content, err := app.vaultManager.ReadRetroActions(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
	})

	mux.HandleFunc("PUT /api/epics/{name}/retro/actions", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := app.vaultManager.WriteRetroActions(name, string(body)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/epics/{name}/retro/actions", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var body struct {
			Description string `json:"description"`
			FromRetro   string `json:"fromRetro,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		action, err := retroSvc.AddAction(name, body.FromRetro, body.Description)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(action)
	})

	mux.HandleFunc("PATCH /api/epics/{name}/retro/actions/{id}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		id := r.PathValue("id")
		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := retroSvc.UpdateActionStatus(name, id, body.Status); err != nil {
			http.Error(w, err.Error(), actionErrStatus(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/epics/{name}/retro/actions/{id}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		id := r.PathValue("id")
		if err := retroSvc.DeleteAction(name, id); err != nil {
			http.Error(w, err.Error(), actionErrStatus(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/epics/{name}/retro/decisions", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := app.vaultManager.AppendRetroDecision(name, body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/epics/{name}/retro/decisions", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		content, err := app.vaultManager.ReadRetroDecisions(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Write([]byte(content))
	})
}

// actionErrStatus maps retro service errors to HTTP status codes using
// sentinel errors rather than string matching.
func actionErrStatus(err error) int {
	switch {
	case errors.Is(err, retroPkg.ErrActionNotFound):
		return http.StatusNotFound
	case errors.Is(err, retroPkg.ErrInvalidActionStatus):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
