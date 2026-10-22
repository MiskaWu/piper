package main

import (
	"net/http"
	"os/exec"
	"strings"

	"github.com/miskawu/piper/internal/pty"
)

func (app *App) registerWSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws/host-shell", func(w http.ResponseWriter, r *http.Request) {
		cmd := exec.Command("bash", "-l")
		pty.HandleTerminalWSWithCmd(w, r, cmd)
	})

	mux.HandleFunc("/ws/status", func(w http.ResponseWriter, r *http.Request) {
		app.hub.HandleWebSocket(w, r)
	})

	mux.HandleFunc("/ws/terminal/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if app.acker != nil {
			app.acker.AckSession(name)
		}
		pty.HandleTerminalWS(w, r, name)
	})

	// Catch-all: static files
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "/" && !strings.Contains(path, ".") {
			r.URL.Path = "/"
		}
		app.fileServer.ServeHTTP(w, r)
	})
}
