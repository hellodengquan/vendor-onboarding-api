package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/internal/storage"
	"vendor-onboarding-api/internal/validators"
	"vendor-onboarding-api/pkg/utils"
)

type FileServiceV2 struct {
	storage storage.Storage
	qv      *validators.QualificationValidator
}

func NewFileServiceV2(store storage.Storage) *FileServiceV2 {
	return &FileServiceV2{storage: store, qv: validators.NewQualificationValidator()}
}

func (s *FileServiceV2) Default() *FileServiceV2 {
	store, err := storage.NewStorageFromEnv()
	if err != nil {
		panic(err)
	}
	return NewFileServiceV2(store)
}

func (s *FileServiceV2) Storage() storage.Storage { return s.storage }

type FileUploadResult struct {
	Qualification *models.Qualification `json:"qualification"`
	StorageKey    string                `json:"storage_key"`
	Provider      string                `json:"provider"`
	ProcessResult *ProcessResult        `json:"process_result,omitempty"`
	Hash          string                `json:"hash"`
	OriginalSize  int64                 `json:"original_size"`
	VirusScan     *VirusScanInfo        `json:"virus_scan,omitempty"`
}

type VirusScanInfo struct {
	ScanSkipped   bool     `json:"scan_skipped"`
	Degraded      bool     `json:"degraded"`
	DegradeReason string   `json:"degrade_reason,omitempty"`
	RateLimited   bool     `json:"rate_limited"`
	QueuedMs      int64    `json:"queued_ms"`
	Threats       []string `json:"threats,omitempty"`
	ScanTimeMs    int64    `json:"scan_time_ms"`
	ScannerName   string   `json:"scanner_name"`
}

func (s *FileServiceV2) UploadQualificationFile(
	ctx context.Context,
	vendorID uint64,
	file *multipart.FileHeader,
	qType models.QualificationType,
	name string,
	number string,
	issuedBy string,
	issuedDate *time.Time,
	expiryDate *time.Time,
	opts ProcessOptions,
) (*FileUploadResult, error) {
	db := database.GetDB()

	var vendor models.Vendor
	if err := db.First(&vendor, vendorID).Error; err != nil {
		return nil, errors.New("供应商不存在")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if err := s.qv.ValidateUpload(qType, file.Size, ext); err != nil {
		return nil, err
	}

	tmpPath, err := s.saveToTemp(file)
	if err != nil {
		return nil, fmt.Errorf("tmp save: %w", err)
	}
	defer os.Remove(tmpPath)

	hash, err := utils.SHA256File(tmpPath)
	if err != nil {
		return nil, err
	}

	processResult := &ProcessResult{}

	var virusScanInfo *VirusScanInfo
	if opts.SkipVirusScan {
		virusScanInfo = &VirusScanInfo{ScanSkipped: true, ScannerName: "skipped"}
	} else {
		scanResult, err := ScanVirusWithContext(ctx, tmpPath, ext)
		if err != nil {
			return nil, fmt.Errorf("virus scan error: %w", err)
		}
		virusScanInfo = &VirusScanInfo{
			Degraded:      scanResult.Degraded,
			DegradeReason: scanResult.DegradeReason,
			RateLimited:   scanResult.RateLimited,
			QueuedMs:      scanResult.QueuedMs,
			Threats:       scanResult.Threats,
			ScanTimeMs:    scanResult.ScanTimeMs,
			ScannerName:   scanResult.ScannerName,
		}
		if !scanResult.Clean {
			return nil, fmt.Errorf("安全扫描发现风险: %v", scanResult.Threats)
		}
	}
	processResult.Applied = append(processResult.Applied, StepVirusScan)

	processedPath := tmpPath
	if isImageExt(ext) && !opts.SkipImageCrop {
		cropDst := tmpPath + ".crop" + ext
		applied, err := CropAndResizeImage(tmpPath, cropDst, opts.MaxImageWidth, opts.MaxImageHeight, opts.ImageQuality)
		if err != nil {
			return nil, fmt.Errorf("image crop: %w", err)
		}
		if applied {
			processedPath = cropDst
			processResult.CropApplied = true
			processResult.Applied = append(processResult.Applied, StepImageCrop)
			defer os.Remove(cropDst)
			newHash, _ := utils.SHA256File(cropDst)
			if newHash != "" {
				hash = newHash
			}
		}
	}
	if ext == ".pdf" && !opts.SkipPDFSanitize {
		sanDst := tmpPath + ".san.pdf"
		applied, err := SanitizePDF(tmpPath, sanDst)
		if err == nil && applied {
			processedPath = sanDst
			processResult.Applied = append(processResult.Applied, StepPDFSanitize)
			defer os.Remove(sanDst)
			newHash, _ := utils.SHA256File(sanDst)
			if newHash != "" {
				hash = newHash
			}
		}
	}

	dateDir := time.Now().Format("2006/01/02")
	base := fmt.Sprintf("%s%s", hash[:16], ext)
	key := fmt.Sprintf("qualifications/%d/%s/%s", vendorID, dateDir, base)

	fp, err := os.Open(processedPath)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	stat, _ := fp.Stat()
	contentType := detectMIME(ext)

	info, err := s.storage.Put(ctx, key, fp, stat.Size(), contentType)
	if err != nil {
		return nil, fmt.Errorf("storage put: %w", err)
	}
	processResult.NewSize = info.Size

	qual := &models.Qualification{
		VendorID:   vendorID,
		Type:       qType,
		Name:       name,
		FileURL:    s.storage.PublicURL(key),
		FileHash:   hash,
		Number:     number,
		IssuedBy:   issuedBy,
		IssuedDate: issuedDate,
		ExpiryDate: expiryDate,
		Status:     models.QualificationStatusPending,
	}
	if err := db.Create(qual).Error; err != nil {
		s.storage.Delete(ctx, key)
		return nil, fmt.Errorf("save qual record: %w", err)
	}

	return &FileUploadResult{
		Qualification: qual,
		StorageKey:    key,
		Provider:      s.storage.Name(),
		ProcessResult: processResult,
		Hash:          hash,
		OriginalSize:  file.Size,
		VirusScan:     virusScanInfo,
	}, nil
}

func (s *FileServiceV2) saveToTemp(fh *multipart.FileHeader) (string, error) {
	src, err := fh.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "voa-*"+filepath.Ext(fh.Filename))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	return tmp.Name(), nil
}

func isImageExt(ext string) bool {
	e := strings.ToLower(ext)
	return e == ".jpg" || e == ".jpeg" || e == ".png" || e == ".gif" || e == ".bmp" || e == ".tiff"
}

func detectMIME(ext string) string {
	switch strings.ToLower(ext) {
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
	}
	return "application/octet-stream"
}
