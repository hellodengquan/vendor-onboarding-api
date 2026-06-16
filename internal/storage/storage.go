package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
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

type DualWriteMode int

const (
	DualWritePrimary    DualWriteMode = iota
	DualWriteBoth                   // 写入双后端，读取走 primary
	DualWriteShadowOnly             // 只写 secondary，不影响 primary 读取
)

type DualWriteStats struct {
	PrimaryWrites   int64
	SecondaryWrites int64
	SecondaryFails  int64
	ShadowReads     int64
	ShadowMismatch  int64
}

type DualWriteStorage struct {
	primary   Storage
	secondary Storage
	mode      DualWriteMode
	shadowCheck bool
	stats     DualWriteStats
	onSecondaryFail func(key string, err error)
}

func NewDualWriteStorage(primary, secondary Storage, mode DualWriteMode) *DualWriteStorage {
	return &DualWriteStorage{
		primary:   primary,
		secondary: secondary,
		mode:      mode,
		onSecondaryFail: func(key string, err error) {},
	}
}

func (d *DualWriteStorage) SetShadowCheck(enabled bool) {
	d.shadowCheck = enabled
}

func (d *DualWriteStorage) SetSecondaryFailHandler(fn func(key string, err error)) {
	if fn != nil {
		d.onSecondaryFail = fn
	}
}

func (d *DualWriteStorage) Stats() DualWriteStats {
	return d.stats
}

func (d *DualWriteStorage) Name() string {
	return fmt.Sprintf("dualwrite[primary=%s,secondary=%s,mode=%d]",
		d.primary.Name(), d.secondary.Name(), d.mode)
}

func (d *DualWriteStorage) writeSecondary(ctx context.Context, key string, action string, fn func(Storage) error) {
	if d.mode == DualWritePrimary {
		return
	}
	if err := fn(d.secondary); err != nil {
		atomic.AddInt64(&d.stats.SecondaryFails, 1)
		d.onSecondaryFail(key+":"+action, err)
		return
	}
	atomic.AddInt64(&d.stats.SecondaryWrites, 1)
}

func (d *DualWriteStorage) Put(ctx context.Context, key string, src io.Reader, size int64, contentType string) (*FileInfo, error) {
	buf, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}

	var fi *FileInfo
	if d.mode != DualWriteShadowOnly {
		fi, err = d.primary.Put(ctx, key, bytes.NewReader(buf), size, contentType)
		if err != nil {
			return nil, err
		}
		atomic.AddInt64(&d.stats.PrimaryWrites, 1)
	}

	d.writeSecondary(ctx, key, "put", func(s Storage) error {
		_, e := s.Put(ctx, key, bytes.NewReader(buf), size, contentType)
		return e
	})

	if fi == nil {
		return d.secondary.Put(ctx, key, bytes.NewReader(buf), size, contentType)
	}
	return fi, nil
}

func (d *DualWriteStorage) Get(ctx context.Context, key string) (io.ReadCloser, *FileInfo, error) {
	rc, fi, err := d.primary.Get(ctx, key)
	if err != nil {
		return rc, fi, err
	}

	if d.shadowCheck && d.secondary != nil {
		atomic.AddInt64(&d.stats.ShadowReads, 1)
		go func(k string, expectedSize int64) {
			_, sfi, serr := d.secondary.Get(ctx, k)
			if serr != nil || (sfi != nil && sfi.Size != expectedSize) {
				atomic.AddInt64(&d.stats.ShadowMismatch, 1)
			}
		}(key, fi.Size)
	}
	return rc, fi, nil
}

func (d *DualWriteStorage) Delete(ctx context.Context, key string) error {
	if d.mode != DualWriteShadowOnly {
		if err := d.primary.Delete(ctx, key); err != nil {
			return err
		}
	}
	d.writeSecondary(ctx, key, "delete", func(s Storage) error {
		return s.Delete(ctx, key)
	})
	return nil
}

func (d *DualWriteStorage) Exists(ctx context.Context, key string) (bool, error) {
	return d.primary.Exists(ctx, key)
}

func (d *DualWriteStorage) PublicURL(key string) string {
	return d.primary.PublicURL(key)
}

func (d *DualWriteStorage) KeyFromURL(publicURL string) (string, error) {
	return d.primary.KeyFromURL(publicURL)
}

func (d *DualWriteStorage) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	return d.primary.List(ctx, prefix)
}

func (d *DualWriteStorage) Primary() Storage  { return d.primary }
func (d *DualWriteStorage) Secondary() Storage { return d.secondary }
