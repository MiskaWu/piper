package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/miskawu/piper/internal/browse"
)

func (app *App) registerFileRoutes(mux *http.ServeMux) {
	fileOp := func(op func(browse.CopyRequest) (browse.FileOpResult, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var req browse.CopyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}

			result, err := op(req)
			if err != nil {
				var ve *browse.ValidationError
				if errors.As(err, &ve) {
					http.Error(w, err.Error(), http.StatusBadRequest)
				} else {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
		}
	}

	mux.HandleFunc("POST /api/files/copy", fileOp(browse.CopyFiles))
	mux.HandleFunc("POST /api/files/move", fileOp(browse.MoveFiles))
}
