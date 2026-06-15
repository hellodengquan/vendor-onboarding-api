package services

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/pkg/utils"
)

var (
	allowedMIMETypes = map[string]bool{
		"application/pdf":       true,
		"image/jpeg":            true,
		"image/jpg":             true,
		"image/png":             true,
		"image/gif":             true,
		"image/bmp":             true,
		"image/tiff":            true,
		"application/msword":    true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/vnd.ms-excel": true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
	}

	allowedExtensions = map[string]bool{
		".pdf":  true,
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".bmp":  true,
		".tiff": true,
		".doc":  true,
		".docx": true,
		".xls":  true,
		".xlsx": true,
	}
)

type FileService struct {
	uploadDir string
	publicURL string
	maxSize   int64
}

func NewFileService() *FileService {
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	publicURL := os.Getenv("PUBLIC_URL")
	if publicURL == "" {
		publicURL = "http://localhost:8080"
	}
	maxSizeStr := os.Getenv("MAX_FILE_SIZE")
	maxSize := int64(10 * 1024 * 1024)
	if maxSizeStr != "" {
		if v, err := strconv.ParseInt(maxSizeStr, 10, 64); err == nil {
			maxSize = v
		}
	}
	return &FileService{
		uploadDir: uploadDir,
		publicURL: publicURL,
		maxSize:   maxSize,
	}
}

type UploadQualificationResult struct {
	Qualification *models.Qualification `json:"qualification"`
	FileHash      string                `json:"file_hash"`
	FileSize      int64                 `json:"file_size"`
}

func (s *FileService) ensureDir(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}

func (s *FileService) validateFileHeader(header *multipart.FileHeader) error {
	if header.Size == 0 {
		return errors.New("文件为空")
	}
	if header.Size > s.maxSize {
		return fmt.Errorf("文件大小超过限制, 最大允许 %d MB", s.maxSize/(1024*1024))
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtensions[ext] {
		return fmt.Errorf("不支持的文件类型: %s", ext)
	}
	return nil
}

func (s *FileService) detectMIME(ext string) string {
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".bmp":
		return "image/bmp"
	case ".tiff":
		return "image/tiff"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	default:
		return "application/octet-stream"
	}
}

func (s *FileService) UploadQualificationFile(
	vendorID uint64,
	file *multipart.FileHeader,
	qType models.QualificationType,
	name string,
	number string,
	issuedBy string,
	issuedDate *time.Time,
	expiryDate *time.Time,
) (*UploadQualificationResult, error) {
	db := database.GetDB()

	var vendor models.Vendor
	if err := db.First(&vendor, vendorID).Error; err != nil {
		return nil, errors.New("供应商不存在")
	}

	if err := s.validateFileHeader(file); err != nil {
		return nil, err
	}

	dateDir := time.Now().Format("2006/01/02")
	targetDir := filepath.Join(s.uploadDir, "qualifications", fmt.Sprintf("%d", vendorID), dateDir)
	if err := s.ensureDir(targetDir); err != nil {
		return nil, fmt.Errorf("创建存储目录失败: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	fileHash, err := s.computeFileHash(file)
	if err != nil {
		return nil, fmt.Errorf("计算文件哈希失败: %w", err)
	}

	fileName := fmt.Sprintf("%s%s", fileHash[:16], ext)
	targetPath := filepath.Join(targetDir, fileName)

	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	dst, err := os.Create(targetPath)
	if err != nil {
		return nil, fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer dst.Close()

	fileSize, err := io.Copy(dst, src)
	if err != nil {
		os.Remove(targetPath)
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}

	relativeURL := fmt.Sprintf("/uploads/qualifications/%d/%s/%s", vendorID, dateDir, fileName)
	fullURL := fmt.Sprintf("%s%s", strings.TrimRight(s.publicURL, "/"), relativeURL)

	qual := &models.Qualification{
		VendorID:   vendorID,
		Type:       qType,
		Name:       name,
		FileURL:    fullURL,
		FileHash:   fileHash,
		Number:     number,
		IssuedBy:   issuedBy,
		IssuedDate: issuedDate,
		ExpiryDate: expiryDate,
		Status:     models.QualificationStatusPending,
	}

	if err := db.Create(qual).Error; err != nil {
		os.Remove(targetPath)
		return nil, fmt.Errorf("保存资质记录失败: %w", err)
	}

	return &UploadQualificationResult{
		Qualification: qual,
		FileHash:      fileHash,
		FileSize:      fileSize,
	}, nil
}

func (s *FileService) computeFileHash(fileHeader *multipart.FileHeader) (string, error) {
	f, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("qtmp_%d_%s", fileHeader.Size, fileHeader.Filename))
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)
	defer tmp.Close()

	if _, err := io.Copy(tmp, f); err != nil {
		return "", err
	}
	tmp.Close()
	return utils.SHA256File(tmpPath)
}

func (s *FileService) GetUploadDir() string {
	return s.uploadDir
}

func (s *FileService) GetLocalPath(fileURL string) (string, error) {
	u, err := url.Parse(fileURL)
	if err != nil {
		return "", err
	}
	path := u.Path
	if strings.HasPrefix(path, "/uploads/") {
		rel := strings.TrimPrefix(path, "/uploads/")
		return filepath.Join(s.uploadDir, rel), nil
	}
	return "", errors.New("无法解析的文件路径")
}
