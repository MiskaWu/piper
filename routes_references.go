package main

import (
	"io"
	"net/http"

	"github.com/miskawu/piper/internal/epiccontainer"
)

// registerReferenceRoutes serves embedded reference content that plan/dispatch/harvest
// sessions fetch on demand. These are kept out of the main session prompt to avoid
// bloating context with rules that only apply in specific flows.
func (app *App) registerReferenceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/references/story-splitting", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = io.WriteString(w, epiccontainer.StorySplittingRule)
	})
}
