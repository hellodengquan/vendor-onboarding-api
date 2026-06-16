package models

import (
	"time"
)

type QualificationType string

const (
	QualificationTypeBusinessLicense   QualificationType = "BUSINESS_LICENSE"
	QualificationTypeTaxCertificate    QualificationType = "TAX_CERTIFICATE"
	QualificationTypeBankAccount       QualificationType = "BANK_ACCOUNT"
	QualificationTypeIDCard            QualificationType = "ID_CARD"
	QualificationTypeOrganizationCode  QualificationType = "ORGANIZATION_CODE"
	QualificationTypeOther             QualificationType = "OTHER"
)

type QualificationStatus string

const (
	QualificationStatusPending   QualificationStatus = "PENDING"
	QualificationStatusVerified  QualificationStatus = "VERIFIED"
	QualificationStatusRejected  QualificationStatus = "REJECTED"
	QualificationStatusExpired   QualificationStatus = "EXPIRED"
)

type Qualification struct {
	ID                 uint64              `gorm:"primaryKey;autoIncrement" json:"id"`
	VendorID           uint64              `gorm:"not null;index" json:"vendor_id"`
	Type               QualificationType   `gorm:"size:32;not null;index" json:"type"`
	Name               string              `gorm:"size:256;not null" json:"name"`
	FileURL            string              `gorm:"size:512" json:"file_url"`
	FileHash           string              `gorm:"size:128" json:"file_hash"`
	Number             string              `gorm:"size:128" json:"number"`
	IssuedBy           string              `gorm:"size:256" json:"issued_by"`
	IssuedDate         *time.Time          `json:"issued_date"`
	ExpiryDate         *time.Time          `json:"expiry_date"`
	Status             QualificationStatus `gorm:"size:32;not null;default:PENDING;index" json:"status"`
	VerifyRemark       string              `gorm:"type:text" json:"verify_remark"`
	VerifiedBy         uint64              `json:"verified_by"`
	VerifiedAt         *time.Time          `json:"verified_at"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

type QualificationUploadRequest struct {
	VendorID   uint64            `json:"vendor_id"`
	Type       QualificationType `json:"type" binding:"required"`
	Name       string            `json:"name" binding:"required"`
	FileURL    string            `json:"file_url" binding:"required"`
	Number     string            `json:"number"`
	IssuedBy   string            `json:"issued_by"`
	IssuedDate *time.Time        `json:"issued_date"`
	ExpiryDate *time.Time        `json:"expiry_date"`
}

type QualificationVerifyRequest struct {
	Status       QualificationStatus `json:"status" binding:"required,oneof=VERIFIED REJECTED"`
	VerifyRemark string              `json:"verify_remark"`
}

type ComplianceCheckResult struct {
	VendorID          uint64   `json:"vendor_id"`
	IsCompliant       bool     `json:"is_compliant"`
	PassedItems       []string `json:"passed_items"`
	FailedItems       []string `json:"failed_items"`
	WarningItems      []string `json:"warning_items"`
	CheckTime         time.Time `json:"check_time"`
}
