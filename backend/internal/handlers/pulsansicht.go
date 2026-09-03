// The endpoint polled by the system view every second.
//
// It combines three sources that say little on their own: requests in the
// last minute, the state of the DB connection pool, and the process memory.
//
// The connection pool is the main reason for this view. Without tuning pgx
// allows as many connections as CPU cores, and when they are all used every
// further request waits — from outside this looks like a slow database even
// though the DB is idle. Confusing the two costs hours of debugging; the
// waiting time is shown here.
package handlers

import (
	"net/http"
	"runtime"
	"time"

	"nexora/internal/middleware"
)

func (s *Server) PulsAnsicht(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r.Context(), middleware.UserID(r)) {
		writeErr(w, http.StatusForbidden, "nur für Administratoren")
		return
	}

	antwort := map[string]interface{}{}

	if s.Puls != nil {
		antwort["anfragen"] = s.Puls.Lies()
	}

	// Der Vorrat an Datenbankverbindungen.
	st := s.Pool.Stat()

	// The average wait time for a connection, not the number of waiters.
	//
	// `EmptyAcquireCount` increments on every attempt that found no free
	// connection, including early startup accesses while the pool builds
	// connections. That count therefore shows non-zero even on a fresh
	// instance and alarmingly coloring it would trigger false alerts.
	//
	// The average wait time avoids that problem: the cost of building a few
	// connections is absorbed, while genuine contention where every request
	// waits raises the average immediately.
	var mittelWarteMS float64
	if st.AcquireCount() > 0 {
		mittelWarteMS = float64(st.AcquireDuration().Microseconds()) /
			float64(st.AcquireCount()) / 1000
	}
	antwort["vorrat"] = map[string]interface{}{
		"hoechstens":    st.MaxConns(),
		"offen":         st.TotalConns(),
		"inBenutzung":   st.AcquiredConns(),
		"frei":          st.IdleConns(),
		"zugriffe":      st.AcquireCount(),
		"ohneFreie":     st.EmptyAcquireCount(),
		"mittelWarteMs": mittelWarteMS,
		"imAufbau":      st.ConstructingConns(),
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	antwort["prozess"] = map[string]interface{}{
		"aufgaben":    runtime.NumGoroutine(),
		"speicherMB":  float64(m.HeapAlloc) / 1024 / 1024,
		"vomSystemMB": float64(m.Sys) / 1024 / 1024,
		"aufraeumen":  m.NumGC,
		"kerne":       runtime.NumCPU(),
	}

	// Database statistics about itself. Two numbers that together indicate
	// whether more RAM would help: while the hit rate is high PostgreSQL
	// already serves from memory.
	ctx := r.Context()
	var groesse string
	var quote *float64
	var verbindungen int
	_ = s.Pool.QueryRow(ctx,
		`SELECT pg_size_pretty(pg_database_size(current_database()))`).Scan(&groesse)
	_ = s.Pool.QueryRow(ctx, `
		SELECT round(100.0*sum(blks_hit)/nullif(sum(blks_hit)+sum(blks_read),0), 2)
		  FROM pg_stat_database WHERE datname = current_database()`).Scan(&quote)
	_ = s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()`).Scan(&verbindungen)

	antwort["datenbank"] = map[string]interface{}{
		"groesse":      groesse,
		"trefferquote": quote,
		"verbindungen": verbindungen,
	}
	antwort["gemessenUm"] = time.Now()

	writeJSON(w, http.StatusOK, antwort)
}
