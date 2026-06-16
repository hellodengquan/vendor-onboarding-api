package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
)

type S3Config struct {
	Bucket    string
	Region    string
	Endpoint  string
	AccessKey string
	SecretKey string
	PublicURL string
	UseSSL    bool
}

type S3Storage struct {
	cfg   S3Config
	emulate bool
	local  *LocalStorage
}

func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("s3 bucket required")
	}

	emulate := false
	if cfg.Region == "" && cfg.AccessKey == "" {
		emulate = true
	}

	var local *LocalStorage
	if emulate {
		root := os.Getenv("S3_EMULATE_DIR")
		if root == "" {
			root = "./s3_emulate/" + cfg.Bucket
		}
		pub := cfg.PublicURL
		if pub == "" {
			pub = "http://localhost:8080"
		}
		local, _ = NewLocalStorage(root, pub, "/s3/"+cfg.Bucket)
	}

	return &S3Storage{cfg: cfg, emulate: emulate, local: local}, nil
}

func (s *S3Storage) Name() string { return "s3" }

func (s *S3Storage) Put(ctx context.Context, key string, src io.Reader, size int64, contentType string) (*FileInfo, error) {
	if s.emulate {
		info, err := s.local.Put(ctx, key, src, size, contentType)
		if err != nil {
			return nil, err
		}
		info.ETag = strings.ReplaceAll(info.ETag, "\"", "")
		return info, nil
	}
	return nil, fmt.Errorf("real S3 client requires AWS SDK (aws-sdk-go-v2) - please install github.com/aws/aws-sdk-go-v2/service/s3")
}

func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, *FileInfo, error) {
	if s.emulate {
		return s.local.Get(ctx, key)
	}
	return nil, nil, errors.New("real S3 Get not implemented in min build")
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	if s.emulate {
		return s.local.Delete(ctx, key)
	}
	return nil
}

func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	if s.emulate {
		return s.local.Exists(ctx, key)
	}
	return false, nil
}

func (s *S3Storage) PublicURL(key string) string {
	if s.emulate {
		return s.local.PublicURL(key)
	}
	if s.cfg.PublicURL != "" {
		return strings.TrimRight(s.cfg.PublicURL, "/") + "/" + strings.TrimLeft(key, "/")
	}
	scheme := "https"
	if !s.cfg.UseSSL {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s.s3.%s.amazonaws.com/%s", scheme, s.cfg.Bucket, s.cfg.Region, key)
}

func (s *S3Storage) KeyFromURL(publicURL string) (string, error) {
	u, err := url.Parse(publicURL)
	if err != nil {
		return "", err
	}
	if s.emulate {
		return s.local.KeyFromURL(publicURL)
	}
	path := strings.TrimLeft(u.Path, "/")
	prefix := s.cfg.Bucket + "/"
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix), nil
	}
	return path, nil
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	if s.emulate {
		return s.local.List(ctx, prefix)
	}
	return nil, fmt.Errorf("real S3 List requires aws-sdk-go-v2/service/s3 (paginated ListObjectsV2)")
}

func EnvS3Config() S3Config {
	useSSL := true
	if v := os.Getenv("S3_USE_SSL"); v == "0" || v == "false" {
		useSSL = false
	}
	return S3Config{
		Bucket:    os.Getenv("S3_BUCKET"),
		Region:    os.Getenv("S3_REGION"),
		Endpoint:  os.Getenv("S3_ENDPOINT"),
		AccessKey: os.Getenv("S3_ACCESS_KEY"),
		SecretKey: os.Getenv("S3_SECRET_KEY"),
		PublicURL: os.Getenv("S3_PUBLIC_URL"),
		UseSSL:    useSSL,
	}
}

type StorageProvider string

const (
	ProviderLocal StorageProvider = "local"
	ProviderS3    StorageProvider = "s3"
)

func NewStorageFromEnv() (Storage, error) {
	provider := StorageProvider(os.Getenv("STORAGE_PROVIDER"))
	switch provider {
	case ProviderS3:
		return NewS3Storage(EnvS3Config())
	default:
		root := os.Getenv("UPLOAD_DIR")
		if root == "" {
			root = "./uploads"
		}
		pub := os.Getenv("PUBLIC_URL")
		if pub == "" {
			pub = "http://localhost:8080"
		}
		return NewLocalStorage(root, pub, "/uploads")
	}
}

var _ = time.Now
