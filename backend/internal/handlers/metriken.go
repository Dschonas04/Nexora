// Der Weg für Prometheus, und damit für Grafana.
//
// Die Systemansicht zeigt die letzte Minute. Das beantwortet "was ist gerade",
// nicht "was war heute Nacht um drei" — und genau das ist die Frage, die man
// hat, wenn sich jemand über gestern beschwert. Dafür braucht es einen, der
// mitschreibt, und das ist nicht die Aufgabe einer Wiki-Anwendung: sie liefert
// die Zahlen, das Aufheben und Zeichnen übernimmt, wer das kann.
//
// Ausgegeben wird im Textformat von Prometheus, von Hand zusammengesetzt. Die
// Bibliothek dafür zöge ein gutes Dutzend Pakete nach sich, und was hier
// entsteht, sind zwanzig Zeilen aus Zahlen, die ohnehin schon vorliegen.
//
// ZÄHLER MÜSSEN STEIGEN. Prometheus bildet die Rate aus zwei Abfragen; ein Wert,
// der zwischendurch kleiner wird, gilt dort als Neustart des Dienstes und
// erzeugt einen Ausschlag, den es nie gab. Die Fächer der letzten Minute sind
// deshalb hier nicht zu gebrauchen, sondern nur die Zähler über die Laufzeit.
package handlers

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"nexora/internal/lizenz"
)

// Metriken beantwortet /metrics.
//
// Ohne hinterlegtes Losungswort gibt es diesen Weg nicht, und zwar mit 404 und
// nicht mit 401: die Zahlen verraten, wie viele Leute hier arbeiten und wann,
// und dass es sie gibt, braucht niemand zu erfahren, der sie nicht abholen darf.
func (s *Server) Metriken(w http.ResponseWriter, r *http.Request) {
	token := MetrikenToken()
	if token == "" {
		http.NotFound(w, r)
		return
	}
	// Vergleich in fester Zeit. Ein gewöhnlicher Vergleich bricht beim ersten
	// falschen Zeichen ab, und daraus lässt sich das Losungswort Zeichen für
	// Zeichen erraten.
	angeboten := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(angeboten), []byte(token)) != 1 {
		http.NotFound(w, r)
		return
	}

	// Vermerken, dass abgeholt wurde. Die Verwaltung zeigt es an: ohne die
	// Gegenprobe weiß niemand, ob die Verdrahtung sitzt, und man sucht den
	// Fehler abwechselnd auf beiden Seiten.
	metrikenAbgeholt()

	var b strings.Builder
	zeile := func(name, art, hilfe string, wert interface{}, marken string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, hilfe, name, art)
		if marken != "" {
			fmt.Fprintf(&b, "%s{%s} %v\n", name, marken, wert)
		} else {
			fmt.Fprintf(&b, "%s %v\n", name, wert)
		}
	}
	weitere := func(name, wert, marken string) {
		fmt.Fprintf(&b, "%s{%s} %s\n", name, marken, wert)
	}

	// ── Anfragen ────────────────────────────────────────────────────────
	if s.Puls != nil {
		gesamt, abgelehnt, fehler, dauer, laufend := s.Puls.SeitDemStart()
		gut := gesamt - abgelehnt - fehler
		zeile("nexora_anfragen_total", "counter",
			"Beantwortete Anfragen seit dem Start, nach Ausgang.", gut, `ergebnis="gut"`)
		weitere("nexora_anfragen_total", fmt.Sprint(abgelehnt), `ergebnis="abgelehnt"`)
		weitere("nexora_anfragen_total", fmt.Sprint(fehler), `ergebnis="fehler"`)
		zeile("nexora_anfragen_dauer_sekunden_summe", "counter",
			"Aufsummierte Bearbeitungsdauer aller Anfragen.", dauer.Seconds(), "")
		zeile("nexora_anfragen_laufend", "gauge",
			"Anfragen, die gerade bearbeitet werden.", laufend, "")
	}

	// ── Der Verbindungsvorrat ───────────────────────────────────────────
	//
	// Die Stelle, an der eine Instanz zuerst eng wird. In Grafana ist das der
	// Graph, den man aufhebt: benutzte Verbindungen gegen die Obergrenze, und
	// daneben die Wartezeit. Berühren sich die beiden ersten Linien, während die
	// dritte steigt, ist die Sache erklärt.
	st := s.Pool.Stat()
	zeile("nexora_vorrat_verbindungen", "gauge",
		"Verbindungen zur Datenbank.", st.AcquiredConns(), `zustand="benutzt"`)
	weitere("nexora_vorrat_verbindungen", fmt.Sprint(st.IdleConns()), `zustand="frei"`)
	weitere("nexora_vorrat_verbindungen", fmt.Sprint(st.MaxConns()), `zustand="hoechstens"`)
	zeile("nexora_vorrat_zugriffe_total", "counter",
		"Zugriffe auf den Vorrat seit dem Start.", st.AcquireCount(), "")
	zeile("nexora_vorrat_ohne_freie_total", "counter",
		"Zugriffe, die keine freie Verbindung vorfanden. Die ersten davon sind das Aufwaermen des Vorrats.",
		st.EmptyAcquireCount(), "")
	zeile("nexora_vorrat_wartezeit_sekunden_summe", "counter",
		"Aufsummierte Wartezeit auf eine Verbindung.", st.AcquireDuration().Seconds(), "")

	// ── Der Prozess ─────────────────────────────────────────────────────
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	zeile("nexora_prozess_speicher_bytes", "gauge",
		"Belegter Speicher auf dem Haufen.", m.HeapAlloc, "")
	zeile("nexora_prozess_aufgaben", "gauge",
		"Nebenlaeufige Aufgaben (Goroutinen).", runtime.NumGoroutine(), "")

	// ── Der Bestand ─────────────────────────────────────────────────────
	//
	// Nicht als Zaehler, sondern als Stand: Seiten koennen auch weniger werden.
	ctx := r.Context()
	zahl := func(sql string) int64 {
		var n int64
		_ = s.Pool.QueryRow(ctx, sql).Scan(&n)
		return n
	}
	zeile("nexora_seiten", "gauge", "Seiten ausserhalb des Papierkorbs.",
		zahl(`SELECT count(*) FROM pages WHERE deleted_at IS NULL`), "")
	zeile("nexora_seiten_papierkorb", "gauge", "Seiten im Papierkorb.",
		zahl(`SELECT count(*) FROM pages WHERE deleted_at IS NOT NULL`), "")
	zeile("nexora_konten", "gauge", "Konten insgesamt.",
		zahl(`SELECT count(*) FROM users`), "")
	zeile("nexora_sitzungen_offen", "gauge", "Nicht widerrufene, noch gueltige Sitzungen.",
		zahl(`SELECT count(*) FROM sitzungen WHERE widerrufen_am IS NULL AND laeuft_ab > now()`), "")
	zeile("nexora_anhaenge", "gauge", "Anhaenge insgesamt.",
		zahl(`SELECT count(*) FROM attachments`), "")
	zeile("nexora_anhaenge_bytes", "gauge", "Belegter Platz durch Anhaenge.",
		zahl(`SELECT coalesce(sum(size), 0) FROM attachments`), "")

	// ── Die Datenbank ueber sich selbst ─────────────────────────────────
	var dbBytes int64
	var quote *float64
	_ = s.Pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&dbBytes)
	_ = s.Pool.QueryRow(ctx, `
		SELECT round(100.0*sum(blks_hit)/nullif(sum(blks_hit)+sum(blks_read),0), 2)
		  FROM pg_stat_database WHERE datname = current_database()`).Scan(&quote)
	zeile("nexora_datenbank_bytes", "gauge", "Groesse der Datenbank.", dbBytes, "")
	if quote != nil {
		zeile("nexora_datenbank_treffer_prozent", "gauge",
			"Anteil der Leseanfragen, die aus dem Speicher beantwortet wurden. Unter 95 lohnt mehr shared_buffers.",
			*quote, "")
	}

	// ── Anmeldungen ─────────────────────────────────────────────────────
	//
	// Fehlversuche gehoeren in ein Diagramm, das jemand ansieht. Eine Spitze
	// darin ist der Unterschied zwischen "jemand hat sein Passwort vergessen"
	// und "jemand arbeitet eine Liste ab".
	zeile("nexora_anmeldungen_total", "counter",
		"Anmeldeversuche, nach Ausgang.",
		zahl(`SELECT count(*) FROM pruefspur WHERE aktion='`+AktAnmeldung+`'`), `ergebnis="gelungen"`)
	weitere("nexora_anmeldungen_total",
		fmt.Sprint(zahl(`SELECT count(*) FROM pruefspur WHERE aktion='`+AktAnmeldungFehl+`'`)),
		`ergebnis="gescheitert"`)

	// ── Die Lizenz ──────────────────────────────────────────────────────
	gueltig := 0
	if lizenz.Aktuell().Gueltig {
		gueltig = 1
	}
	zeile("nexora_lizenz_gueltig", "gauge",
		"1, wenn ein gueltiger Schluessel hinterlegt ist.", gueltig, "")
	frei := 0
	for _, f := range lizenz.Alle {
		if lizenz.Frei(f) {
			frei++
		}
	}
	zeile("nexora_funktionen_frei", "gauge",
		"Anzahl freigeschalteter Zusaetze.", frei, "")

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
