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
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
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

// RedisTLS describes whether and how the connection is encrypted. Nil means
// unencrypted, which is acceptable for a cache on the same host.
//
// It must be encrypted when the cache is reachable over a network: session
// tokens live there and anyone reading them is effectively signed in.
type RedisTLS struct {
	Wurzeln *x509.CertPool
	// Name is the name in the certificate. Empty means: use the hostname
	// from the address, which is correct when the service is addressed by
	// its name.
	Name string
}

// NeuRedis connects. A failure here must never stop the service: the
// application runs perfectly well without Redis.
func NeuRedis(ctx context.Context, adresse, passwort string, datenbank int, vorsilbe string, sicher *RedisTLS) *RedisSpeicher {
	adresse = strings.TrimSpace(adresse)
	if adresse == "" {
		return nil
	}
	optionen := &redis.Options{
		Addr:     adresse,
		Password: passwort,
		DB:       datenbank,
		// Short deadlines: a hanging cache must not hold up a request. Better
		// to answer without it than to wait for it.
		DialTimeout:  2 * time.Second,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	if sicher != nil {
		name := sicher.Name
		if name == "" {
			if wirt, _, err := net.SplitHostPort(adresse); err == nil {
				name = wirt
			} else {
				name = adresse
			}
		}
		optionen.TLSConfig = &tls.Config{
			RootCAs:    sicher.Wurzeln,
			ServerName: name,
			MinVersion: tls.VersionTLS12,
		}
	}
	c := redis.NewClient(optionen)
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
	weg := "offen"
	if sicher != nil {
		weg = "verschlüsselt"
	}
	log.Printf("Redis: %s (%s), Namensraum %s", adresse, weg, vorsilbe)
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
