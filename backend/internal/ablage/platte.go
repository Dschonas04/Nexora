package ablage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Platte keeps every attachment as one file in a directory. This is what
// Nexora has always done and stays the default: it needs no other service, and
// for a single container it is perfectly adequate.
type Platte struct {
	Verzeichnis string
}

func NeuePlatte(verzeichnis string) *Platte { return &Platte{Verzeichnis: verzeichnis} }

func (p *Platte) Name() string { return "Platte (" + p.Verzeichnis + ")" }

func (p *Platte) pfad(key string) string {
	// filepath.Base cuts off any path part in the key. The keys are UUIDs we
	// generate ourselves, but this line is cheaper than the certainty that it
	// stays that way forever.
	return filepath.Join(p.Verzeichnis, filepath.Base(key))
}

func (p *Platte) Schreiben(ctx context.Context, key string, r io.Reader, _ int64, _ string) (int64, error) {
	if err := os.MkdirAll(p.Verzeichnis, 0o755); err != nil {
		return 0, fmt.Errorf("Verzeichnis anlegen: %w", err)
	}
	ziel := p.pfad(key)
	f, err := os.Create(ziel)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, r)
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		// Clear away the fragment. Otherwise half a file lies there that a row in
		// the database believes in.
		os.Remove(ziel)
		return 0, err
	}
	return n, nil
}

func (p *Platte) Lesen(ctx context.Context, key string) (io.ReadCloser, error) {
	return os.Open(p.pfad(key))
}

func (p *Platte) Loeschen(ctx context.Context, key string) error {
	err := os.Remove(p.pfad(key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
