package storage

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	rootDir   string
	publicURL string
	urlPrefix string
}

func NewLocalStorage(rootDir string, publicURL string, urlPrefix string) (*LocalStorage, error) {
	if rootDir == "" {
		rootDir = "./uploads"
	}
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, err
	}
	return &LocalStorage{
		rootDir:   rootDir,
		publicURL: strings.TrimRight(publicURL, "/"),
		urlPrefix: urlPrefix,
	}, nil
}

func (s *LocalStorage) Name() string { return "local" }

func (s *LocalStorage) absPath(key string) string {
	return filepath.Join(s.rootDir, filepath.FromSlash(key))
}

func (s *LocalStorage) ensureDir(p string) error {
	return os.MkdirAll(filepath.Dir(p), 0755)
}

func (s *LocalStorage) Put(ctx context.Context, key string, src io.Reader, size int64, contentType string) (*FileInfo, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	target := s.absPath(key)
	if err := s.ensureDir(target); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	h := md5.New()
	tee := io.TeeReader(src, h)

	f, err := os.Create(target)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	n, err := io.Copy(f, tee)
	if err != nil {
		os.Remove(target)
		return nil, err
	}
	info, _ := f.Stat()
	return &FileInfo{
		Key:          key,
		Size:         n,
		ContentType:  contentType,
		ETag:         "\"" + hex.EncodeToString(h.Sum(nil)) + "\"",
		LastModified: info.ModTime(),
	}, nil
}

func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, *FileInfo, error) {
	target := s.absPath(key)
	f, err := os.Open(target)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	info := &FileInfo{
		Key:          key,
		Size:         st.Size(),
		LastModified: st.ModTime(),
	}
	return f, info, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	target := s.absPath(key)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(target)
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	target := s.absPath(key)
	_, err := os.Stat(target)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalStorage) PublicURL(key string) string {
	path := strings.TrimRight(s.urlPrefix, "/") + "/" + strings.TrimLeft(key, "/")
	return s.publicURL + path
}

func (s *LocalStorage) KeyFromURL(publicURL string) (string, error) {
	u, err := url.Parse(publicURL)
	if err != nil {
		return "", err
	}
	path := u.Path
	prefix := s.urlPrefix
	if prefix == "" {
		prefix = "/uploads"
	}
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix+"/"), nil
	}
	return "", errors.New("url does not belong to this storage")
}
