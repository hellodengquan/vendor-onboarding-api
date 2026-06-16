package database

import (
	"fmt"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DataSourceRegistry struct {
	dbs map[string]*gorm.DB
	tpm *TwoPhaseManager
}

var Registry *DataSourceRegistry

func InitMultiSource() error {
	Registry = &DataSourceRegistry{dbs: make(map[string]*gorm.DB)}

	mainDSN := os.Getenv("MAIN_DB_DSN")
	auditDSN := os.Getenv("AUDIT_DB_DSN")

	var mainDB, auditDB *gorm.DB
	var err error

	if mainDSN != "" {
		mainDB, err = gorm.Open(postgres.Open(mainDSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		})
	} else {
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "vendor_onboarding.db"
		}
		mainDB, err = gorm.Open(sqlite.Open(dbPath+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		})
	}
	if err != nil {
		return fmt.Errorf("open main db: %w", err)
	}

	DB = mainDB
	Registry.dbs["main"] = mainDB

	if auditDSN != "" {
		auditDB, err = gorm.Open(postgres.Open(auditDSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		})
		if err != nil {
			return fmt.Errorf("open audit db: %w", err)
		}
		Registry.dbs["audit"] = auditDB
	} else {
		auditPath := os.Getenv("AUDIT_DB_PATH")
		if auditPath == "" {
			auditPath = "vendor_audit.db"
		}
		auditDB, err = gorm.Open(sqlite.Open(auditPath+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Info),
		})
		if err != nil {
			return fmt.Errorf("open audit db: %w", err)
		}
		Registry.dbs["audit"] = auditDB
	}

	Registry.tpm = NewTwoPhaseManager(30 * time.Second)
	_ = Registry.tpm.Register("main", mainDB)
	_ = Registry.tpm.Register("audit", auditDB)

	return nil
}

func (r *DataSourceRegistry) Get(name string) *gorm.DB {
	return r.dbs[name]
}

func (r *DataSourceRegistry) TPM() *TwoPhaseManager {
	return r.tpm
}

func GetAuditDB() *gorm.DB {
	if Registry == nil {
		return DB
	}
	return Registry.Get("audit")
}
