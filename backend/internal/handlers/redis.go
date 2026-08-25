// Redis as a shared cache.
//
// The division of labour is deliberate: PostgreSQL holds the truth, Redis holds
// a fast copy. Everything kept here can be rebuilt from the database at any
// time, which is why a Redis outage is not an outage of the application, only a
// slower one.
//
// That is exactly the difference from the common design of keeping sessions in
// Redis alone: there a restart of the cache signs everyone out, and losing it
// means nobody can say who was signed in.
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
	// A prefix in front of every key, so two Nexora instances can share one
	// Redis without overwriting each other's entries.
	vorsilbe string
}

// NeuRedis connects. A failure here must never stop the service: the
// application runs perfectly well without Redis.
func NeuRedis(ctx context.Context, adresse, passwort string, datenbank int, vorsilbe string) *RedisSpeicher {
	adresse = strings.TrimSpace(adresse)
	if adresse == "" {
		return nil
	}
	c := redis.NewClient(&redis.Options{
		Addr:     adresse,
		Password: passwort,
		DB:       datenbank,
		// Short deadlines: a hanging cache must not hold up a request. Better
		// to answer without it than to wait for it.
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
		// A revocation is remembered longer than a validity: it is the
		// statement that matters, and it will not change again.
		dauer = 10 * time.Minute
	}
	s.Redis.client.Set(ctx, s.Redis.schluessel("sitzung", sid), wert, dauer)
}

// ZaehlerHoch counts an event and returns the tally inside the window.
//
// It allows sign-in attempts to be rate limited without keeping a table for it.
// Without Redis it returns 0, which the caller treats as "no limit": a lockout
// that fires at random depending on whether a cache happens to be there would
// be worse than none at all.
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
