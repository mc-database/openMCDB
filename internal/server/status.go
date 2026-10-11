package server

import (
	"fmt"
	"log"
	"net/http"
)

func (s *Server) showStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.db.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage failure")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, err = fmt.Fprintf(w, `%d`, n)
	if err != nil {
		log.Printf("openmcdb: status write failed: %v", err)
		return
	}
}
