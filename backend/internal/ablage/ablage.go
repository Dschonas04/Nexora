// Package ablage stores attachment files.
//
// Two implementations: the local disk, and any S3-compatible object store. The
// interface exists so the handlers never learn which one is in use — a page
// upload looks the same whether the bytes land in /data or in a bucket.
//
// The split matters for more than tidiness. Attachments are the one part of
// Nexora that lives outside the database, so they are also the part that makes
// backups awkward and horizontal scaling impossible: two containers cannot
// share a local directory. Moving them into an object store removes both
// problems at once.
package ablage

import (
	"context"
	"io"
)

// Ablage is what the handlers use. Keys are attachment ids, opaque strings; the
// implementations decide what a key means on their side.
type Ablage interface {
	// Schreiben stores the stream under key and returns how many bytes landed.
	// A failed write must leave nothing behind — a half-written attachment that
	// the database believes in is worse than a failed upload.
	Schreiben(ctx context.Context, key string, r io.Reader, groesse int64, mime string) (int64, error)

	// Lesen opens the object. The caller closes it.
	Lesen(ctx context.Context, key string) (io.ReadCloser, error)

	// Loeschen removes it. A missing object is not an error: deleting something
	// that is already gone is the outcome the caller wanted.
	Loeschen(ctx context.Context, key string) error

	// Name is what the startup log and the settings page show.
	Name() string
}
