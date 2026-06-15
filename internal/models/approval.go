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
	ApprovalStatusPartial  ApprovalStatus = "PARTIAL"
)

type SignType string

const (
	SignTypeSingle   SignType = "SINGLE"
	SignTypeAny      SignType = "ANY"
	SignTypeAll      SignType = "ALL"
	SignTypeMajority SignType = "MAJORITY"
)

type ApprovalFlow struct {
	ID            uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	VendorID      uint64         `gorm:"not null;uniqueIndex" json:"vendor_id"`
	CurrentStage  ApprovalStage  `gorm:"size:32;not null;index" json:"current_stage"`
	StageOrder    int            `gorm:"not null;default:0" json:"stage_order"`
	Status        ApprovalStatus `gorm:"size:32;not null;default:PENDING" json:"status"`
	ApprovedCount int            `gorm:"not null;default:0" json:"approved_count"`
	RejectedCount int            `gorm:"not null;default:0" json:"rejected_count"`
	SignerCount   int            `gorm:"not null;default:0" json:"signer_count"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type ApprovalNodeConfig struct {
	ID           uint64        `gorm:"primaryKey;autoIncrement" json:"id"`
	Stage        ApprovalStage `gorm:"size:32;not null;index" json:"stage"`
	StageOrder   int           `gorm:"not null;index" json:"stage_order"`
	StageName    string        `gorm:"size:128;not null" json:"stage_name"`
	SignType     SignType      `gorm:"size:16;not null;default:ALL" json:"sign_type"`
	IsEnabled    bool          `gorm:"not null;default:true" json:"is_enabled"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type ApprovalNodeSigner struct {
	ID             uint64        `gorm:"primaryKey;autoIncrement" json:"id"`
	ApprovalNodeID uint64        `gorm:"not null;index" json:"approval_node_id"`
	ApproverID     uint64        `gorm:"not null;index" json:"approver_id"`
	ApproverName   string        `gorm:"size:64;not null" json:"approver_name"`
	ApproverRole   string        `gorm:"size:64" json:"approver_role"`
	Stage          ApprovalStage `gorm:"size:32;index" json:"stage"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type ApprovalRecord struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	VendorID     uint64         `gorm:"not null;index" json:"vendor_id"`
	Stage        ApprovalStage  `gorm:"size:32;not null;index" json:"stage"`
	StageOrder   int            `gorm:"not null" json:"stage_order"`
	Status       ApprovalStatus `gorm:"size:32;not null" json:"status"`
	ApproverID   uint64         `gorm:"not null;index" json:"approver_id"`
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

type NodeConfigCreateRequest struct {
	Stage     ApprovalStage `json:"stage" binding:"required"`
	StageName string        `json:"stage_name" binding:"required"`
	SignType  SignType      `json:"sign_type" binding:"required,oneof=SINGLE ANY ALL MAJORITY"`
	Signers   []NodeSignerItem `json:"signers" binding:"required,min=1"`
}

type NodeSignerItem struct {
	ApproverID   uint64 `json:"approver_id" binding:"required"`
	ApproverName string `json:"approver_name" binding:"required"`
	ApproverRole string `json:"approver_role"`
}

type FlowDetailResponse struct {
	Flow          ApprovalFlow        `json:"flow"`
	Records       []ApprovalRecord    `json:"records"`
	Vendor        Vendor              `json:"vendor"`
	CurrentSigners []ApprovalNodeSigner `json:"current_signers,omitempty"`
	NodeConfigs   []ApprovalNodeConfig `json:"node_configs,omitempty"`
}

type StageSignStatus struct {
	Stage         ApprovalStage       `json:"stage"`
	StageName     string              `json:"stage_name"`
	SignType      SignType            `json:"sign_type"`
	TotalSigners  int                 `json:"total_signers"`
	ApprovedCount int                 `json:"approved_count"`
	RejectedCount int                 `json:"rejected_count"`
	IsComplete    bool                `json:"is_complete"`
	Signers       []ApprovalNodeSigner `json:"signers"`
	Records       []ApprovalRecord    `json:"records"`
}
