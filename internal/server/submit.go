package server

import (
	"errors"
	"io"
	"log"
	"net/http"

	"openmcdb/internal/store"
)

const maxSubmitBytes = 64 * 1024

func (s *Server) submitSnapshot(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSubmitBytes)

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			log.Printf("submit: body too large (limit %d bytes)", maxSubmitBytes)
			writeError(w, http.StatusRequestEntityTooLarge, "payload too large")
			return
		}
		log.Printf("submit: read body: %v", err)
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	snap, from, err := store.UpgradeToLatest(raw)
	if err != nil {
		log.Printf("submit: upgrade schema=%d: %v", from, err)
		switch {
		case errors.Is(err, store.ErrSchemaTooOld),
			errors.Is(err, store.ErrSchemaTooNew),
			errors.Is(err, store.ErrSchemaMissing):
			writeError(w, http.StatusBadRequest, "unsupported schema")
		default:
			writeError(w, http.StatusBadRequest, "invalid snapshot")
		}
		return
	}
	if err := s.db.Insert(r.Context(), snap); err != nil {
		if errors.Is(err, store.ErrInvalidSnapshot) {
			log.Printf("submit: invalid snapshot: %v", err)
			writeError(w, http.StatusBadRequest, "invalid snapshot")
			return
		}
		log.Printf("submit: insert: %v", err)
		writeError(w, http.StatusInternalServerError, "storage failure")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
