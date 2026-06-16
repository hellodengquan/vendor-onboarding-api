package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/models"
)

type APIKeyService struct {
	cancelCleanup context.CancelFunc
	cleanupWG     sync.WaitGroup
	cleanupMu     sync.Mutex
}

func NewAPIKeyService() *APIKeyService {
	return &APIKeyService{}
}

func (s *APIKeyService) Create(appKey string, remark string) (*models.APIKey, string, error) {
	db := database.GetDB()
	if appKey == "" {
		buf := make([]byte, 10)
		rand.Read(buf)
		appKey = "ak_" + hex.EncodeToString(buf)
	}

	var existing models.APIKey
	if err := db.Where("app_key = ?", appKey).First(&existing).Error; err == nil {
		return nil, "", errors.New("app_key 已存在")
	}

	secret, err := generateSecret(32)
	if err != nil {
		return nil, "", err
	}

	key := &models.APIKey{
		AppKey:    appKey,
		AppSecret: secret,
		Status:    "ACTIVE",
		Version:   1,
		Remark:    remark,
	}
	if err := db.Create(key).Error; err != nil {
		return nil, "", err
	}
	return key, secret, nil
}

func (s *APIKeyService) List() ([]models.APIKey, error) {
	db := database.GetDB()
	var list []models.APIKey
	if err := db.Order("id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		list[i].AppSecret = maskSecret(list[i].AppSecret)
	}
	return list, nil
}

func (s *APIKeyService) Get(appKey string) (*models.APIKey, error) {
	db := database.GetDB()
	var key models.APIKey
	if err := db.Where("app_key = ? AND status = ?", appKey, "ACTIVE").First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}

func (s *APIKeyService) GetActiveSecrets(appKey string) (map[int]string, error) {
	db := database.GetDB()
	var keys []models.APIKey
	now := time.Now()

	query := db.Where("app_key = ? AND status IN ? AND (expire_at IS NULL OR expire_at > ?)",
		appKey, []string{"ACTIVE", "ROTATING"}, now)
	if err := query.Find(&keys).Error; err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	result := make(map[int]string, len(keys))
	for _, k := range keys {
		result[k.Version] = k.AppSecret
	}
	return result, nil
}

func (s *APIKeyService) Rotate(appKey string, oldExpireHours int, remark string) (*models.APIKey, string, error) {
	db := database.GetDB()

	if oldExpireHours <= 0 {
		oldExpireHours = 24
	}

	var current models.APIKey
	if err := db.Where("app_key = ? AND status = ?", appKey, "ACTIVE").First(&current).Error; err != nil {
		return nil, "", errors.New("未找到有效的密钥")
	}

	newSecret, err := generateSecret(32)
	if err != nil {
		return nil, "", err
	}

	tx := db.Begin()

	expireAt := time.Now().Add(time.Duration(oldExpireHours) * time.Hour)
	current.Status = "ROTATING"
	current.ExpireAt = &expireAt
	if err := tx.Save(&current).Error; err != nil {
		tx.Rollback()
		return nil, "", err
	}

	newKey := &models.APIKey{
		AppKey:    appKey,
		AppSecret: newSecret,
		Status:    "ACTIVE",
		Version:   current.Version + 1,
		Remark:    fmt.Sprintf("%s [旋转自 v%d]", remark, current.Version),
	}
	if err := tx.Create(newKey).Error; err != nil {
		tx.Rollback()
		return nil, "", err
	}

	tx.Commit()
	return newKey, newSecret, nil
}

func (s *APIKeyService) Revoke(appKey string, version int) error {
	db := database.GetDB()
	return db.Model(&models.APIKey{}).
		Where("app_key = ? AND version = ?", appKey, version).
		Updates(map[string]interface{}{
			"status":   "REVOKED",
			"expire_at": time.Now(),
		}).Error
}

func (s *APIKeyService) CleanupExpired() (int64, error) {
	db := database.GetDB()
	res := db.Where("status = ? AND expire_at IS NOT NULL AND expire_at < ?", "ROTATING", time.Now()).
		Update("status", "EXPIRED")
	return res.RowsAffected, res.Error
}

func (s *APIKeyService) CleanupExpiredFull(gracePeriodDays int) (rotatedExpired int64, revokedPurged int64, err error) {
	db := database.GetDB()

	if gracePeriodDays <= 0 {
		gracePeriodDays = 90
	}
	cutoff := time.Now().AddDate(0, 0, -gracePeriodDays)

	res1 := db.Where("status = ? AND expire_at IS NOT NULL AND expire_at < ?", "ROTATING", time.Now()).
		Update("status", "EXPIRED")
	if res1.Error != nil {
		return 0, 0, res1.Error
	}
	rotatedExpired = res1.RowsAffected

	res2 := db.Where("status IN ? AND updated_at < ?",
		[]string{"EXPIRED", "REVOKED"}, cutoff).
		Delete(&models.APIKey{})
	if res2.Error != nil {
		return rotatedExpired, 0, res2.Error
	}
	revokedPurged = res2.RowsAffected
	return rotatedExpired, revokedPurged, nil
}

func (s *APIKeyService) StartAutoCleanup(ctx context.Context, interval time.Duration, gracePeriodDays int) {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()

	if s.cancelCleanup != nil {
		s.cancelCleanup()
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancelCleanup = cancel

	s.cleanupWG.Add(1)
	go func() {
		defer s.cleanupWG.Done()
		if interval <= 0 {
			interval = 1 * time.Hour
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				_, _, _ = s.CleanupExpiredFull(gracePeriodDays)
			}
		}
	}()
}

func (s *APIKeyService) StopAutoCleanup() {
	s.cleanupMu.Lock()
	cancel := s.cancelCleanup
	s.cancelCleanup = nil
	s.cleanupMu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.cleanupWG.Wait()
}

func generateSecret(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sk_" + hex.EncodeToString(buf), nil
}

func maskSecret(secret string) string {
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "****" + secret[len(secret)-4:]
}
