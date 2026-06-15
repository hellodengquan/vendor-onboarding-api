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
	)
	if err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	log.Println("Database initialized successfully")
	return nil
}

func GetDB() *gorm.DB {
	return DB
}
