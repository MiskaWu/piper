package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

func (app *App) registerGitRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/git/available", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{
			"available": app.gitFetcher.Available(),
		})
	})

	mux.HandleFunc("GET /api/git/branches", func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("path")
		if dir == "" {
			http.Error(w, "path query parameter required", http.StatusBadRequest)
			return
		}
		result, err := app.gitFetcher.GetBranches(dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("GET /api/git/graph", func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("path")
		if dir == "" {
			http.Error(w, "path query parameter required", http.StatusBadRequest)
			return
		}
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}
		ref := r.URL.Query().Get("ref")
		var hideRemotes []string
		if hr := r.URL.Query().Get("hideRemotes"); hr != "" {
			for _, s := range strings.Split(hr, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					hideRemotes = append(hideRemotes, s)
				}
			}
		}
		result, err := app.gitFetcher.GetGraphRef(dir, limit, ref, hideRemotes...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if app.gitWatcher != nil {
			if wErr := app.gitWatcher.Watch(dir); wErr != nil {
				log.Printf("WARNING: git watcher failed for %s: %v", dir, wErr)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("GET /api/git/commit-body", func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("path")
		hash := r.URL.Query().Get("hash")
		if dir == "" || hash == "" {
			http.Error(w, "path and hash query parameters required", http.StatusBadRequest)
			return
		}
		detail, err := app.gitFetcher.GetCommitBody(dir, hash)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	})

	mux.HandleFunc("POST /api/git/init", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path      string `json:"path"`
			RemoteURL string `json:"remoteUrl,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Path == "" {
			http.Error(w, "path is required", http.StatusBadRequest)
			return
		}
		if err := app.gitFetcher.InitRepo(body.Path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if body.RemoteURL != "" {
			if err := app.gitFetcher.AddRemote(body.Path, "origin", body.RemoteURL); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"isRepo": true,
		})
	})

	mux.HandleFunc("POST /api/git/remote", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Path == "" || body.Name == "" || body.URL == "" {
			http.Error(w, "path, name, and url are required", http.StatusBadRequest)
			return
		}
		if err := app.gitFetcher.AddRemote(body.Path, body.Name, body.URL); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/git/push", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path   string `json:"path"`
			Branch string `json:"branch"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Path == "" {
			http.Error(w, "path is required", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		hasKeys, err := app.gitFetcher.CheckSSHAgent()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "ssh_error",
				"code":    "SSH_AGENT_UNAVAILABLE",
				"message": "SSH agent is not running or unreachable. Open a host terminal and run: ssh-add",
			})
			return
		}
		if !hasKeys {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "ssh_error",
				"code":    "SSH_NO_KEYS",
				"message": "SSH agent has no keys loaded. Open a host terminal and run: ssh-add",
			})
			return
		}

		if err := app.gitFetcher.Push(body.Path, body.Branch); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "error",
				"code":    "PUSH_FAILED",
				"message": err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
		})
	})

	mux.HandleFunc("POST /api/git/tag", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
			Name string `json:"name"`
			Hash string `json:"hash"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Path == "" || body.Name == "" || body.Hash == "" {
			http.Error(w, "path, name, and hash are required", http.StatusBadRequest)
			return
		}
		if err := app.gitFetcher.CreateTag(body.Path, body.Name, body.Hash); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"name":   body.Name,
			"hash":   body.Hash,
		})
	})

	mux.HandleFunc("DELETE /api/git/tag", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Path == "" || body.Name == "" {
			http.Error(w, "path and name are required", http.StatusBadRequest)
			return
		}
		if err := app.gitFetcher.DeleteTag(body.Path, body.Name); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/git/tag/push", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path   string `json:"path"`
			Name   string `json:"name"`
			Remote string `json:"remote"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Path == "" || body.Name == "" {
			http.Error(w, "path and name are required", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		hasKeys, err := app.gitFetcher.CheckSSHAgent()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "ssh_error",
				"code":    "SSH_AGENT_UNAVAILABLE",
				"message": "SSH agent is not running or unreachable. Open a host terminal and run: ssh-add",
			})
			return
		}
		if !hasKeys {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "ssh_error",
				"code":    "SSH_NO_KEYS",
				"message": "SSH agent has no keys loaded. Open a host terminal and run: ssh-add",
			})
			return
		}

		if err := app.gitFetcher.PushTag(body.Path, body.Remote, body.Name); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "error",
				"code":    "PUSH_FAILED",
				"message": err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
		})
	})

	mux.HandleFunc("GET /api/git/tags", func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("path")
		if dir == "" {
			http.Error(w, "path query parameter required", http.StatusBadRequest)
			return
		}
		tags, err := app.gitFetcher.ListTags(dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tags)
	})

	mux.HandleFunc("GET /api/git/file-status", func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("path")
		if dir == "" {
			http.Error(w, "path query parameter required", http.StatusBadRequest)
			return
		}
		statuses, err := app.gitChecker.GetFileStatuses(dir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statuses)
	})

	mux.HandleFunc("GET /api/git/submodules", func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("path")
		if dir == "" {
			http.Error(w, "path query parameter required", http.StatusBadRequest)
			return
		}
		subs, err := app.gitFetcher.ListSubmodules(dir)
		if err != nil {
			// Not a git repo or no submodules — return empty list
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]struct{}{})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(subs)
	})

	mux.HandleFunc("POST /api/git/submodules/push", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path  string   `json:"path"`
			Paths []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Path == "" || len(body.Paths) == 0 {
			http.Error(w, "path and paths are required", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		hasKeys, err := app.gitFetcher.CheckSSHAgent()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "ssh_error",
				"code":    "SSH_AGENT_UNAVAILABLE",
				"message": "SSH agent is not running or unreachable. Open a host terminal and run: ssh-add",
			})
			return
		}
		if !hasKeys {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "ssh_error",
				"code":    "SSH_NO_KEYS",
				"message": "SSH agent has no keys loaded. Open a host terminal and run: ssh-add",
			})
			return
		}

		if err := app.gitFetcher.PushSubmodules(body.Path, body.Paths); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "error",
				"code":    "PUSH_FAILED",
				"message": err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
		})
	})
}
