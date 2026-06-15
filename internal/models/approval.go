package models

import (
	"time"
)

type ApprovalStage string

const (
	StageDataCollection  ApprovalStage = "DATA_COLLECTION"
	StageComplianceCheck ApprovalStage = "COMPLIANCE_CHECK"
	StageLevel1Approval  ApprovalStage = "LEVEL_1_APPROVAL"
	StageLevel2Approval  ApprovalStage = "LEVEL_2_APPROVAL"
	StageCompleted       ApprovalStage = "COMPLETED"
	StageRejected        ApprovalStage = "REJECTED"
)

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "PENDING"
	ApprovalStatusApproved ApprovalStatus = "APPROVED"
	ApprovalStatusRejected ApprovalStatus = "REJECTED"
)

type ApprovalFlow struct {
	ID            uint64        `gorm:"primaryKey;autoIncrement" json:"id"`
	VendorID      uint64        `gorm:"not null;uniqueIndex" json:"vendor_id"`
	CurrentStage  ApprovalStage `gorm:"size:32;not null;index" json:"current_stage"`
	StageOrder    int           `gorm:"not null;default:0" json:"stage_order"`
	Status        ApprovalStatus `gorm:"size:32;not null;default:PENDING" json:"status"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

type ApprovalRecord struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	VendorID     uint64         `gorm:"not null;index" json:"vendor_id"`
	Stage        ApprovalStage  `gorm:"size:32;not null;index" json:"stage"`
	StageOrder   int            `gorm:"not null" json:"stage_order"`
	Status       ApprovalStatus `gorm:"size:32;not null" json:"status"`
	ApproverID   uint64         `json:"approver_id"`
	ApproverName string         `gorm:"size:64" json:"approver_name"`
	Remark       string         `gorm:"type:text" json:"remark"`
	ApprovedAt   *time.Time     `json:"approved_at"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type ApprovalRequest struct {
	VendorID uint64 `json:"vendor_id" binding:"required"`
	Remark   string `json:"remark"`
}

type ApprovalRejectRequest struct {
	VendorID uint64 `json:"vendor_id" binding:"required"`
	Remark   string `json:"remark" binding:"required"`
}

type StageTransitionRequest struct {
	VendorID uint64        `json:"vendor_id" binding:"required"`
	ToStage  ApprovalStage `json:"to_stage" binding:"required"`
	Remark   string        `json:"remark"`
}

type FlowDetailResponse struct {
	Flow      ApprovalFlow     `json:"flow"`
	Records   []ApprovalRecord `json:"records"`
	Vendor    Vendor           `json:"vendor"`
}
