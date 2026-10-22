package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/miskawu/piper/internal/vault"
)

// registerVaultRoutes registers vault file CRUD endpoints.
func (app *App) registerVaultRoutes(mux *http.ServeMux) {

	// --- Problem Statement ---

	mux.HandleFunc("GET /api/epics/{name}/problem-statement", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		content, err := app.vaultManager.ReadProblemStatement(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
	})

	mux.HandleFunc("PUT /api/epics/{name}/problem-statement", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		if err := app.vaultManager.WriteProblemStatement(name, string(body)); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "must contain") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("update", "problem-statement", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// --- ADR ---

	mux.HandleFunc("GET /api/epics/{name}/adr", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		names, err := app.vaultManager.ListADRs(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(names)
	})

	mux.HandleFunc("GET /api/epics/{name}/adr/{file}", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		file := r.PathValue("file")
		content, err := app.vaultManager.ReadADR(name, file)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "invalid filename") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
	})

	mux.HandleFunc("PUT /api/epics/{name}/adr/{file}", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		file := r.PathValue("file")
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		if err := app.vaultManager.WriteADR(name, file, string(body)); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "must contain") || strings.Contains(err.Error(), "invalid filename") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("update", "adr", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("DELETE /api/epics/{name}/adr/{file}", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		file := r.PathValue("file")
		if err := app.vaultManager.DeleteADR(name, file); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "invalid filename") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("delete", "adr", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// --- Spikes ---

	mux.HandleFunc("GET /api/epics/{name}/spikes", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		names, err := app.vaultManager.ListSpikes(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(names)
	})

	mux.HandleFunc("GET /api/epics/{name}/spikes/{file}", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		file := r.PathValue("file")
		content, err := app.vaultManager.ReadSpike(name, file)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "invalid filename") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
	})

	mux.HandleFunc("PUT /api/epics/{name}/spikes/{file}", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		file := r.PathValue("file")
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		if err := app.vaultManager.WriteSpike(name, file, string(body)); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "invalid filename") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("update", "spike", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("DELETE /api/epics/{name}/spikes/{file}", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		file := r.PathValue("file")
		if err := app.vaultManager.DeleteSpike(name, file); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "invalid filename") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("delete", "spike", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// --- Specs ---

	mux.HandleFunc("GET /api/epics/{name}/specs/summary", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		content, err := app.vaultManager.ReadSpecsSummary(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
	})

	mux.HandleFunc("PUT /api/epics/{name}/specs/summary", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		if err := app.vaultManager.WriteSpecsSummary(name, string(body)); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("update", "specs-summary", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/epics/{name}/specs/changelog", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		content, err := app.vaultManager.ReadSpecsChangelog(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
	})

	mux.HandleFunc("PUT /api/epics/{name}/specs/changelog", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		if err := app.vaultManager.WriteSpecsChangelog(name, string(body)); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("update", "specs-changelog", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/epics/{name}/specs/extracts", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		names, err := app.vaultManager.ListExtracts(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(names)
	})

	mux.HandleFunc("GET /api/epics/{name}/specs/extracts/{file}", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		file := r.PathValue("file")
		content, err := app.vaultManager.ReadExtract(name, file)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "invalid filename") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(content))
	})

	// --- Contracts ---

	mux.HandleFunc("GET /api/epics/{name}/contracts", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		owner := r.URL.Query().Get("owner")
		var names []string
		var err error
		if owner != "" {
			names, err = app.vaultManager.ListContractsByOwner(name, owner)
		} else {
			names, err = app.vaultManager.ListContracts(name)
		}
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if names == nil {
			names = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(names)
	})

	mux.HandleFunc("GET /api/epics/{name}/contracts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		id := r.PathValue("id")
		content, err := app.vaultManager.ReadContract(name, id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "invalid filename") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Write([]byte(content))
	})

	mux.HandleFunc("PUT /api/epics/{name}/contracts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		id := r.PathValue("id")
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		if err := app.vaultManager.WriteContract(name, id, string(body)); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "invalid filename") ||
				strings.Contains(err.Error(), "missing required") ||
				strings.Contains(err.Error(), "unknown type") ||
				strings.Contains(err.Error(), "requires") ||
				strings.Contains(err.Error(), "parse contract") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("update", "contract", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/epics/{name}/contracts/{id}/generate", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		id := r.PathValue("id")

		var req struct {
			DryRun    bool `json:"dryRun"`
			Overwrite bool `json:"overwrite"`
		}
		// Empty body is OK — both fields default to false.
		if r.ContentLength > 0 {
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
		}

		artifacts, err := app.vaultManager.GenerateContract(name, id, app.resolveProjectPath, vault.GenerateOptions{DryRun: req.DryRun, Overwrite: req.Overwrite})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if strings.Contains(err.Error(), "unknown format") ||
				strings.Contains(err.Error(), "not configured") ||
				strings.Contains(err.Error(), "not supported") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		app.broadcastPlanUpdate("generate", "contract", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "ok",
			"artifacts": artifacts,
		})
	})

	mux.HandleFunc("DELETE /api/epics/{name}/contracts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")
		id := r.PathValue("id")
		if err := app.vaultManager.DeleteContract(name, id); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "invalid filename") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("delete", "contract", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/epics/{name}/specs/extract", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusServiceUnavailable)
			return
		}
		name := r.PathValue("name")

		// Accept JSON body with sourcePath field
		var req struct {
			SourcePath string `json:"sourcePath"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if req.SourcePath == "" {
			http.Error(w, "sourcePath is required", http.StatusBadRequest)
			return
		}

		extractName, extractContent, err := app.vaultManager.RunExtract(name, req.SourcePath)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		app.broadcastPlanUpdate("update", "specs-extract", name)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"name":    extractName,
			"content": extractContent,
		})
	})
}

// broadcastPlanUpdate sends a plan_update WebSocket message.
func (app *App) broadcastPlanUpdate(action, resource, epic string) {
	if app.hub == nil {
		return
	}
	msg, _ := json.Marshal(map[string]string{
		"type":      "plan_update",
		"action":    action,
		"resource":  resource,
		"epic":      epic,
		"timestamp": time.Now().Format(time.RFC3339),
	})
	app.hub.Broadcast(msg)
}
