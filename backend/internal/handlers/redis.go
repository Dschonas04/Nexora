// Redis als geteilter Zwischenspeicher.
//
// Die Rollenverteilung ist ausdrücklich: PostgreSQL hält die Wahrheit, Redis
// hält eine schnelle Kopie. Alles, was hier liegt, lässt sich jederzeit aus der
// Datenbank neu herstellen, deshalb ist ein Ausfall von Redis kein Ausfall
// der Anwendung, sondern nur eine langsamere.
//
// Genau darin liegt der Unterschied zu der verbreiteten Bauweise, Sitzungen
// ausschließlich in Redis zu legen: dort bedeutet ein Neustart des Speichers,
// dass alle abgemeldet sind, und ein Verlust bedeutet, dass niemand mehr sagen
// kann, wer angemeldet war.
package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisSpeicher bundles the connection and the namespace.
type RedisSpeicher struct {
	client *redis.Client
	// Vorsilbe vor jedem Schlüssel, damit sich zwei Nexora-Instanzen eine
	// Redis teilen können, ohne sich die Einträge gegenseitig zu überschreiben.
	vorsilbe string
}

// NeuRedis verbindet sich. Ein Fehler ist keiner, mit dem der Dienst stehen
// bleiben darf: die Anwendung läuft ohne Redis vollständig.
func NeuRedis(ctx context.Context, adresse, passwort string, datenbank int, vorsilbe string) *RedisSpeicher {
	adresse = strings.TrimSpace(adresse)
	if adresse == "" {
		return nil
	}
	c := redis.NewClient(&redis.Options{
		Addr:     adresse,
		Password: passwort,
		DB:       datenbank,
		// Kurze Fristen: ein hängender Zwischenspeicher darf keine Anfrage
		// aufhalten. Lieber ohne ihn antworten als mit ihm warten.
		DialTimeout:  2 * time.Second,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	})
	pruef, abbruch := context.WithTimeout(ctx, 3*time.Second)
	defer abbruch()
	if err := c.Ping(pruef).Err(); err != nil {
		log.Printf("Redis unter %s nicht erreichbar (%v). Nexora läuft ohne ihn weiter.", adresse, err)
		c.Close()
		return nil
	}
	if vorsilbe == "" {
		vorsilbe = "nexora"
	}
	log.Printf("Redis: %s, Namensraum %s", adresse, vorsilbe)
	return &RedisSpeicher{client: c, vorsilbe: vorsilbe}
}

func (r *RedisSpeicher) schluessel(art, id string) string {
	return r.vorsilbe + ":" + art + ":" + id
}

// Schliessen hands the connection back.
func (r *RedisSpeicher) Schliessen() {
	if r != nil && r.client != nil {
		r.client.Close()
	}
}

// redisSitzung reads the remembered state of a session.
func (s *Server) redisSitzung(sid string) (bool, bool) {
	ctx, abbruch := context.WithTimeout(context.Background(), time.Second)
	defer abbruch()
	wert, err := s.Redis.client.Get(ctx, s.Redis.schluessel("sitzung", sid)).Result()
	if err != nil {
		return false, false
	}
	return wert == "1", true
}

// redisSitzungMerken records the state.
func (s *Server) redisSitzungMerken(sid string, gilt bool) {
	ctx, abbruch := context.WithTimeout(context.Background(), time.Second)
	defer abbruch()
	wert := "0"
	dauer := speicherDauer
	if gilt {
		wert = "1"
	} else {
		// Ein Widerruf wird länger festgehalten als eine Gültigkeit: er ist
		// die Aussage, auf die es ankommt, und er ändert sich nicht mehr.
		dauer = 10 * time.Minute
	}
	s.Redis.client.Set(ctx, s.Redis.schluessel("sitzung", sid), wert, dauer)
}

// ZaehlerHoch zählt ein Ereignis und liefert den Stand innerhalb des Fensters.
//
// Damit lassen sich Anmeldeversuche begrenzen, ohne dafür eine Tabelle zu
// führen. Ohne Redis liefert es 0, der Aufrufer behandelt das als "keine
// Begrenzung", denn eine Sperre, die ohne Zwischenspeicher zufällig zuschlägt,
// wäre schlimmer als keine.
func (s *Server) ZaehlerHoch(art, id string, fenster time.Duration) int64 {
	if s.Redis == nil {
		return 0
	}
	ctx, abbruch := context.WithTimeout(context.Background(), time.Second)
	defer abbruch()
	sch := s.Redis.schluessel("zaehler:"+art, id)
	n, err := s.Redis.client.Incr(ctx, sch).Result()
	if err != nil {
		return 0
	}
	if n == 1 {
		s.Redis.client.Expire(ctx, sch, fenster)
	}
	return n
}
