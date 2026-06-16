package database

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vendor-onboarding-api/internal/models"
)

var DB *gorm.DB

func Init() error {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "vendor_onboarding.db"
	}

	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	err = DB.AutoMigrate(
		&models.Vendor{},
		&models.Qualification{},
		&models.ApprovalFlow{},
		&models.ApprovalRecord{},
		&models.ApprovalNodeConfig{},
		&models.ApprovalNodeSigner{},
		&models.ParallelGroup{},
		&models.ParallelGroupInstance{},
		&models.AdditionalSignerRecord{},
		&models.WithdrawalRecord{},
		&models.APIKey{},
	)
	if err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	if err := seedDefaultApprovalNodes(DB); err != nil {
		return fmt.Errorf("failed to seed approval nodes: %w", err)
	}
	if err := seedDefaultAPIKey(DB); err != nil {
		return fmt.Errorf("failed to seed api key: %w", err)
	}

	log.Println("Database initialized successfully")
	return nil
}

func seedDefaultApprovalNodes(db *gorm.DB) error {
	stages := []struct {
		Stage     models.ApprovalStage
		StageName string
		Order     int
		SignType  models.SignType
	}{
		{Stage: models.StageDataCollection, StageName: "资料收集", Order: 0, SignType: models.SignTypeSingle},
		{Stage: models.StageComplianceCheck, StageName: "合规性检查", Order: 1, SignType: models.SignTypeAny},
		{Stage: models.StageLevel1Approval, StageName: "一级审批", Order: 2, SignType: models.SignTypeAll},
		{Stage: models.StageLevel2Approval, StageName: "二级审批", Order: 3, SignType: models.SignTypeMajority},
		{Stage: models.StageFinanceApproval, StageName: "财务审批", Order: 4, SignType: models.SignTypeAny},
		{Stage: models.StageLegalApproval, StageName: "法务审批", Order: 5, SignType: models.SignTypeAny},
	}

	for _, s := range stages {
		var count int64
		db.Model(&models.ApprovalNodeConfig{}).Where("stage = ?", s.Stage).Count(&count)
		if count == 0 {
			node := &models.ApprovalNodeConfig{
				Stage:      s.Stage,
				StageOrder: s.Order,
				StageName:  s.StageName,
				SignType:   s.SignType,
				IsEnabled:  true,
			}
			if err := db.Create(node).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func seedDefaultAPIKey(db *gorm.DB) error {
	var count int64
	db.Model(&models.APIKey{}).Where("app_key = ?", "vendor_admin").Count(&count)
	if count == 0 {
		return db.Create(&models.APIKey{
			AppKey:    "vendor_admin",
			AppSecret: "vendor-onboarding-secret-2024",
			Status:    "ACTIVE",
			Version:   1,
			Remark:    "系统默认管理员密钥",
		}).Error
	}
	return nil
}

func GetDB() *gorm.DB {
	return DB
}
