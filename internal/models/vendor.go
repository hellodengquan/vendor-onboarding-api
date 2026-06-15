package models

import (
	"time"
)

type VendorStatus string

const (
	VendorStatusDraft          VendorStatus = "DRAFT"
	VendorStatusPendingReview  VendorStatus = "PENDING_REVIEW"
	VendorStatusComplianceCheck VendorStatus = "COMPLIANCE_CHECK"
	VendorStatusPendingApproval VendorStatus = "PENDING_APPROVAL"
	VendorStatusApproved       VendorStatus = "APPROVED"
	VendorStatusRejected       VendorStatus = "REJECTED"
)

type Vendor struct {
	ID              uint64       `gorm:"primaryKey;autoIncrement" json:"id"`
	VendorCode      string       `gorm:"uniqueIndex;size:64;not null" json:"vendor_code"`
	CompanyName     string       `gorm:"size:256;not null" json:"company_name"`
	UnifiedSocialCreditCode string `gorm:"size:32;uniqueIndex" json:"unified_social_credit_code"`
	LegalPerson     string       `gorm:"size:64" json:"legal_person"`
	ContactPerson   string       `gorm:"size:64" json:"contact_person"`
	ContactPhone    string       `gorm:"size:32" json:"contact_phone"`
	ContactEmail    string       `gorm:"size:128" json:"contact_email"`
	RegisteredAddress string     `gorm:"size:512" json:"registered_address"`
	BusinessScope   string       `gorm:"type:text" json:"business_scope"`
	Status          VendorStatus `gorm:"size:32;not null;index" json:"status"`
	CurrentStageID  uint64       `gorm:"index" json:"current_stage_id"`
	SubmittedBy     uint64       `json:"submitted_by"`
	SubmittedAt     *time.Time   `json:"submitted_at"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

type VendorCreateRequest struct {
	CompanyName           string `json:"company_name" binding:"required"`
	UnifiedSocialCreditCode string `json:"unified_social_credit_code" binding:"required"`
	LegalPerson           string `json:"legal_person"`
	ContactPerson         string `json:"contact_person" binding:"required"`
	ContactPhone          string `json:"contact_phone" binding:"required"`
	ContactEmail          string `json:"contact_email"`
	RegisteredAddress     string `json:"registered_address"`
	BusinessScope         string `json:"business_scope"`
}

type VendorUpdateRequest struct {
	CompanyName           string `json:"company_name"`
	UnifiedSocialCreditCode string `json:"unified_social_credit_code"`
	LegalPerson           string `json:"legal_person"`
	ContactPerson         string `json:"contact_person"`
	ContactPhone          string `json:"contact_phone"`
	ContactEmail          string `json:"contact_email"`
	RegisteredAddress     string `json:"registered_address"`
	BusinessScope         string `json:"business_scope"`
}

type VendorListRequest struct {
	Page     int          `form:"page,default=1"`
	PageSize int          `form:"page_size,default=20"`
	Status   VendorStatus `form:"status"`
	Keyword  string       `form:"keyword"`
}

type VendorListResponse struct {
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
	List     []Vendor `json:"list"`
}
