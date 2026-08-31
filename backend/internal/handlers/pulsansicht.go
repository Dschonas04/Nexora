// Der Weg, den die Systemansicht im Sekundentakt abfragt.
//
// Er fasst drei Quellen zusammen, die ohne einander wenig sagen: die Anfragen
// der letzten Minute, den Zustand des Verbindungsvorrats zur Datenbank und den
// Speicher des Prozesses.
//
// Der Verbindungsvorrat ist der eigentliche Grund für diese Ansicht. pgx nimmt
// ohne Angabe so viele Verbindungen wie Kerne, und wenn die alle belegt sind,
// wartet jede weitere Anfrage — von außen sieht das aus wie eine langsame
// Datenbank, obwohl die Datenbank Langeweile hat. Diese Verwechslung kostet
// Stunden, wenn man die Wartezeit nicht sieht. Hier steht sie.
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

	// Die mittlere Wartezeit auf eine Verbindung, und nicht die Zahl der
	// Wartenden.
	//
	// EmptyAcquireCount zählt jeden Zugriff, der keine freie Verbindung vorfand,
	// und dazu gehören die ersten Zugriffe nach dem Start: der Vorrat ist dann
	// leer und muss die Verbindung erst aufbauen. Diese Zahl steht also auch auf
	// einer Instanz, die nie unter Last war, bei ein paar Dutzend. Sie rot zu
	// färben hieße, auf jeder frischen Installation Alarm zu schlagen.
	//
	// Die mittlere Wartezeit hat das Problem nicht: das Aufbauen einer Handvoll
	// Verbindungen verschwindet darin, und echte Knappheit, bei der jeder
	// Zugriff wartet, hebt sie sofort an.
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

	// Die Datenbank über sich selbst. Zwei Zahlen, die zusammen die Frage
	// beantworten, ob mehr Arbeitsspeicher etwas brächte: solange die
	// Trefferquote oben steht, liest PostgreSQL ohnehin aus dem Speicher.
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
