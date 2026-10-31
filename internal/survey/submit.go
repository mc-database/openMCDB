package survey

import (
	"errors"
	"io"
	"log"
	"net/http"

	"openmcdb/internal/server"
)

const maxSubmitBytes = 64 * 1024

func (m *Module) submitSnapshot(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSubmitBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			log.Printf("survey: body too large (limit %d bytes)", maxSubmitBytes)
			server.WriteError(w, http.StatusRequestEntityTooLarge, "payload too large")
			return
		}
		log.Printf("survey: read body: %v", err)
		server.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	snap, err := decodeSnapshot(raw)
	if err != nil {
		log.Printf("survey: decode payload: %v", err)
		if errors.Is(err, ErrUnsupportedSchema) {
			server.WriteError(w, http.StatusBadRequest, "unsupported schema")
		} else {
			server.WriteError(w, http.StatusBadRequest, "invalid snapshot")
		}
		return
	}
	if err := m.store.Insert(r.Context(), snap); err != nil {
		if errors.Is(err, ErrInvalidSnapshot) {
			log.Printf("survey: invalid snapshot: %v", err)
			server.WriteError(w, http.StatusBadRequest, "invalid snapshot")
			return
		}
		log.Printf("survey: insert: %v", err)
		server.WriteError(w, http.StatusInternalServerError, "storage failure")
		return
	}
	server.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
