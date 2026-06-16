package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

type FileInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
	Metadata     map[string]string
}

type Storage interface {
	Name() string
	Put(ctx context.Context, key string, src io.Reader, size int64, contentType string) (*FileInfo, error)
	Get(ctx context.Context, key string) (io.ReadCloser, *FileInfo, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	PublicURL(key string) string
	KeyFromURL(publicURL string) (string, error)
	List(ctx context.Context, prefix string) ([]FileInfo, error)
}

type MigrationResult struct {
	Migrated int
	Skipped  int
	Failed   int
	FailedKeys []string
	Elapsed  time.Duration
}

var ErrMigrationInProgress = errors.New("migration already in progress")

func Migrate(ctx context.Context, dst Storage, src Storage, prefix string, deleteAfterCopy bool) (*MigrationResult, error) {
	result := &MigrationResult{}
	start := time.Now()
	defer func() { result.Elapsed = time.Since(start) }()

	files, err := src.List(ctx, prefix)
	if err != nil {
		return nil, err
	}

	for _, fi := range files {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		exists, _ := dst.Exists(ctx, fi.Key)
		if exists {
			result.Skipped++
			continue
		}
		rc, _, err := src.Get(ctx, fi.Key)
		if err != nil {
			result.Failed++
			result.FailedKeys = append(result.FailedKeys, fi.Key)
			continue
		}
		_, err = dst.Put(ctx, fi.Key, rc, fi.Size, fi.ContentType)
		rc.Close()
		if err != nil {
			result.Failed++
			result.FailedKeys = append(result.FailedKeys, fi.Key)
			continue
		}
		result.Migrated++
		if deleteAfterCopy {
			_ = src.Delete(ctx, fi.Key)
		}
	}

	return result, nil
}
