package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/miskawu/piper/internal/summarizer"
	"github.com/miskawu/piper/internal/vault"
)

func (app *App) registerIdeaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/ideas", func(w http.ResponseWriter, r *http.Request) {
		if app.vaultManager == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]*vault.Idea{})
			return
		}
		ideas, err := app.vaultManager.GetIdeas()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get ideas: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ideas)
	})

	mux.HandleFunc("POST /api/ideas", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Text) == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}

		var ctx []vault.IdeaContext
		if app.activityProvider != nil {
			events := app.activityProvider.GetRecentActivity()
			for _, e := range events {
				ctx = append(ctx, vault.IdeaContext{
					Session: e.SessionName,
					Kind:    e.Kind,
					Detail:  e.Detail,
					Time:    e.Time.Unix(),
				})
			}
		}

		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusInternalServerError)
			return
		}
		idea, err := app.vaultManager.AddIdea(req.Text, ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to add idea: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(idea)
	})

	mux.HandleFunc("DELETE /api/ideas/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "invalid idea id", http.StatusBadRequest)
			return
		}
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusInternalServerError)
			return
		}
		if err := app.vaultManager.DeleteIdea(id); err != nil {
			http.Error(w, fmt.Sprintf("failed to delete idea: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	})

	mux.HandleFunc("PUT /api/ideas/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "invalid idea id", http.StatusBadRequest)
			return
		}
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Text) == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusInternalServerError)
			return
		}
		updated, err := app.vaultManager.UpdateIdeaText(id, req.Text)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to update idea: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)
	})

	mux.HandleFunc("POST /api/ideas/{id}/enrich", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "invalid idea id", http.StatusBadRequest)
			return
		}

		if app.vaultManager == nil {
			http.Error(w, "vault not configured", http.StatusInternalServerError)
			return
		}
		idea, err := app.vaultManager.GetIdea(id)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get idea: %v", err), http.StatusInternalServerError)
			return
		}
		if idea == nil {
			http.Error(w, "idea not found", http.StatusNotFound)
			return
		}

		var contextBlock string
		if len(idea.Context) > 0 {
			var lines []string
			for _, c := range idea.Context {
				lines = append(lines, fmt.Sprintf("- [%s] %s: %s", c.Kind, c.Session, c.Detail))
			}
			contextBlock = strings.Join(lines, "\n")
		} else {
			contextBlock = "(無上下文記錄)"
		}

		prompt := fmt.Sprintf(`根據以下靈感筆記和工作上下文，補充為結構化的需求描述。
包含：背景說明、具體需求、驗收條件。
用與筆記相同的語言回覆，簡潔扼要。

## 靈感筆記
%s

## 當時的工作上下文（最近 5 分鐘的活動）
%s`, idea.Text, contextBlock)

		if app.enrichCmd == nil {
			http.Error(w, "enrich not configured", http.StatusInternalServerError)
			return
		}

		enrichModel := app.sessionStore.GetModelConfig().IdeaEnrich
		if enrichModel == "" {
			enrichModel = "sonnet"
		}
		output, err := app.enrichCmd.Output("claude", "-p", "--model", enrichModel, "--output-format", "text", prompt)
		if err != nil {
			http.Error(w, fmt.Sprintf("AI enrich failed: %v", err), http.StatusInternalServerError)
			return
		}

		desc := strings.TrimSpace(string(output))
		if err := app.vaultManager.UpdateIdeaEnrichedDesc(id, desc); err != nil {
			http.Error(w, fmt.Sprintf("failed to save enriched desc: %v", err), http.StatusInternalServerError)
			return
		}

		if app.costLogger != nil {
			app.costLogger.Log(summarizer.CostEntry{
				Timestamp:   time.Now(),
				SessionName: "idea-enrich",
			})
		}

		updated, _ := app.vaultManager.GetIdea(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)
	})
}
