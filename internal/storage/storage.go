package storage

import (
	"context"
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
}
