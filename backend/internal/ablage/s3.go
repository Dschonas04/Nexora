package ablage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3 keeps attachments in an S3-compatible bucket: MinIO, Garage, Ceph, AWS.
//
// The bytes still travel through Nexora on their way to the reader rather than
// being handed out as a presigned link. That is slower, and it is deliberate:
// a presigned URL is valid for anyone who holds it, which would quietly
// sidestep the per-page permission check that decides who may see an
// attachment at all.
type S3 struct {
	klient  *minio.Client
	eimer   string
	anzeige string
}

// Einstellungen are the values from config.conf.
type Einstellungen struct {
	Endpunkt  string // host:port, without a scheme
	Bucket    string
	Zugriff   string
	Geheimnis string
	Region    string
	TLS       bool
	Pfadstil  bool // true: http://host/bucket/key statt http://bucket.host/key
}

// NeuS3 connects and makes sure the bucket exists.
//
// The bucket is created when it is missing: otherwise every installation would
// have to be set up by hand beforehand, and the first upload would fail without
// anybody understanding why.
func NeuS3(ctx context.Context, e Einstellungen) (*S3, error) {
	endpunkt := e.Endpunkt
	// An endpoint accidentally entered with a scheme is the most common typo.
	// minio-go wants it without, so we take it off here instead of punishing the
	// operator with a cryptic message.
	if u, err := url.Parse(endpunkt); err == nil && u.Host != "" {
		endpunkt = u.Host
	}
	endpunkt = strings.TrimSuffix(endpunkt, "/")

	klient, err := minio.New(endpunkt, &minio.Options{
		Creds:        credentials.NewStaticV4(e.Zugriff, e.Geheimnis, ""),
		Secure:       e.TLS,
		Region:       e.Region,
		BucketLookup: bucketArt(e.Pfadstil),
	})
	if err != nil {
		return nil, fmt.Errorf("S3-Verbindung: %w", err)
	}

	da, err := klient.BucketExists(ctx, e.Bucket)
	if err != nil {
		return nil, fmt.Errorf("Eimer %q prüfen: %w", e.Bucket, err)
	}
	if !da {
		if err := klient.MakeBucket(ctx, e.Bucket, minio.MakeBucketOptions{Region: e.Region}); err != nil {
			return nil, fmt.Errorf("Eimer %q anlegen: %w", e.Bucket, err)
		}
	}

	schema := "http"
	if e.TLS {
		schema = "https"
	}
	return &S3{
		klient:  klient,
		eimer:   e.Bucket,
		anzeige: fmt.Sprintf("S3 (%s://%s/%s)", schema, endpunkt, e.Bucket),
	}, nil
}

func bucketArt(pfadstil bool) minio.BucketLookupType {
	if pfadstil {
		return minio.BucketLookupPath
	}
	return minio.BucketLookupAuto
}

func (s *S3) Name() string { return s.anzeige }

func (s *S3) Schreiben(ctx context.Context, key string, r io.Reader, groesse int64, mime string) (int64, error) {
	if groesse <= 0 {
		// Unknown length: minio-go then uploads in parts. That works but needs more
		// memory, so we pass the size along whenever we can.
		groesse = -1
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	info, err := s.klient.PutObject(ctx, s.eimer, key, r, groesse,
		minio.PutObjectOptions{ContentType: mime})
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

func (s *S3) Lesen(ctx context.Context, key string) (io.ReadCloser, error) {
	o, err := s.klient.GetObject(ctx, s.eimer, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// GetObject reports a missing key only on the first read. A Stat beforehand
	// turns the late, incomprehensible error into an early, understandable one.
	if _, err := o.Stat(); err != nil {
		o.Close()
		return nil, err
	}
	return o, nil
}

func (s *S3) Loeschen(ctx context.Context, key string) error {
	err := s.klient.RemoveObject(ctx, s.eimer, key, minio.RemoveObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil
		}
	}
	return err
}
