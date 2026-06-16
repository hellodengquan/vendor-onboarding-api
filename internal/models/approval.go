package models

import (
	"time"
)

type ApprovalStage string

const (
	StageDataCollection   ApprovalStage = "DATA_COLLECTION"
	StageComplianceCheck  ApprovalStage = "COMPLIANCE_CHECK"
	StageLevel1Approval   ApprovalStage = "LEVEL_1_APPROVAL"
	StageLevel2Approval   ApprovalStage = "LEVEL_2_APPROVAL"
	StageFinanceApproval  ApprovalStage = "FINANCE_APPROVAL"
	StageLegalApproval    ApprovalStage = "LEGAL_APPROVAL"
	StageParallelGroup    ApprovalStage = "PARALLEL_GROUP"
	StageCompleted        ApprovalStage = "COMPLETED"
	StageRejected         ApprovalStage = "REJECTED"
	StageWithdrawn        ApprovalStage = "WITHDRAWN"
)

type ApprovalStatus string

const (
	ApprovalStatusPending   ApprovalStatus = "PENDING"
	ApprovalStatusApproved  ApprovalStatus = "APPROVED"
	ApprovalStatusRejected  ApprovalStatus = "REJECTED"
	ApprovalStatusPartial   ApprovalStatus = "PARTIAL"
	ApprovalStatusWithdrawn ApprovalStatus = "WITHDRAWN"
)

type SignType string

const (
	SignTypeSingle   SignType = "SINGLE"
	SignTypeAny      SignType = "ANY"
	SignTypeAll      SignType = "ALL"
	SignTypeMajority SignType = "MAJORITY"
)

type SignerSource string

const (
	SignerSourceConfig   SignerSource = "CONFIG"
	SignerSourceAddition SignerSource = "ADDITION"
	SignerSourceDelegate SignerSource = "DELEGATE"
)

type AdditionSignerMode string

const (
	AddModeBefore     AdditionSignerMode = "BEFORE"
	AddModeAfter      AdditionSignerMode = "AFTER"
	AddModeConcurrent AdditionSignerMode = "CONCURRENT"
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
	ParallelGroupID *uint64      `gorm:"index" json:"parallel_group_id,omitempty"`
	WithdrawnBy     *uint64      `json:"withdrawn_by,omitempty"`
	WithdrawnAt     *time.Time   `json:"withdrawn_at,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
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
	Source         SignerSource  `gorm:"size:16;not null;default:CONFIG" json:"source"`
	Sequence       int           `gorm:"not null;default:0" json:"sequence"`
	AddedBy        *uint64       `json:"added_by,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type ParallelGroup struct {
	ID             uint64        `gorm:"primaryKey;autoIncrement" json:"id"`
	GroupCode      string        `gorm:"size:64;uniqueIndex" json:"group_code"`
	Name           string        `gorm:"size:128;not null" json:"name"`
	SubStages      string        `gorm:"type:text;not null" json:"sub_stages_json"`
	SignType       SignType      `gorm:"size:16;not null;default:ALL" json:"sign_type"`
	JoinStrategy   string        `gorm:"size:16;not null;default:ALL_PASS" json:"join_strategy"`
	IsEnabled      bool          `gorm:"not null;default:true" json:"is_enabled"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type ParallelGroupInstance struct {
	ID             uint64        `gorm:"primaryKey;autoIncrement" json:"id"`
	VendorID       uint64        `gorm:"not null;index" json:"vendor_id"`
	GroupID        uint64        `gorm:"not null;index" json:"group_id"`
	ParentStageOrder int         `gorm:"not null" json:"parent_stage_order"`
	SubStageStatus string        `gorm:"type:text" json:"sub_stage_status_json"`
	IsResolved     bool          `gorm:"not null;default:false" json:"is_resolved"`
	ResolvedAt     *time.Time    `json:"resolved_at,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type AdditionalSignerRecord struct {
	ID             uint64              `gorm:"primaryKey;autoIncrement" json:"id"`
	VendorID       uint64              `gorm:"not null;index" json:"vendor_id"`
	Stage          ApprovalStage       `gorm:"size:32;not null;index" json:"stage"`
	ApproverID     uint64              `gorm:"not null;index" json:"approver_id"`
	ApproverName   string              `gorm:"size:64;not null" json:"approver_name"`
	ApproverRole   string              `gorm:"size:64" json:"approver_role"`
	AddMode        AdditionSignerMode  `gorm:"size:16;not null;default:CONCURRENT" json:"add_mode"`
	AddedBy        uint64              `json:"added_by"`
	AddedByName    string              `gorm:"size:64" json:"added_by_name"`
	Remark         string              `gorm:"type:text" json:"remark"`
	Sequence       int                 `gorm:"not null;default:0" json:"sequence"`
	IsSigned       bool                `gorm:"not null;default:false" json:"is_signed"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

type WithdrawalRecord struct {
	ID             uint64        `gorm:"primaryKey;autoIncrement" json:"id"`
	VendorID       uint64        `gorm:"not null;index" json:"vendor_id"`
	FromStage      ApprovalStage `gorm:"size:32;not null" json:"from_stage"`
	FromStageOrder int           `gorm:"not null" json:"from_stage_order"`
	ToStage        ApprovalStage `gorm:"size:32;not null" json:"to_stage"`
	ToStageOrder   int           `gorm:"not null" json:"to_stage_order"`
	WithdrawnBy    uint64        `json:"withdrawn_by"`
	WithdrawnByName string       `gorm:"size:64" json:"withdrawn_by_name"`
	Reason         string        `gorm:"type:text;not null" json:"reason"`
	IsRollback     bool          `gorm:"not null;default:true" json:"is_rollback"`
	CreatedAt      time.Time     `json:"created_at"`
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
	AdditionalSigners []AdditionalSignerRecord `json:"additional_signers,omitempty"`
}

type AddSignerRequest struct {
	VendorID     uint64              `json:"vendor_id" binding:"required"`
	Stage        ApprovalStage       `json:"stage"`
	ApproverID   uint64              `json:"approver_id" binding:"required"`
	ApproverName string              `json:"approver_name" binding:"required"`
	ApproverRole string              `json:"approver_role"`
	AddMode      AdditionSignerMode  `json:"add_mode" binding:"required,oneof=BEFORE AFTER CONCURRENT"`
	Sequence     int                 `json:"sequence"`
	Remark       string              `json:"remark"`
}

type WithdrawRequest struct {
	VendorID   uint64        `json:"vendor_id" binding:"required"`
	ToStage    ApprovalStage `json:"to_stage"`
	Reason     string        `json:"reason" binding:"required"`
	IsRollback bool          `json:"is_rollback"`
}

type ParallelGroupCreateRequest struct {
	GroupCode    string          `json:"group_code" binding:"required"`
	Name         string          `json:"name" binding:"required"`
	SubStages    []ApprovalStage `json:"sub_stages" binding:"required,min=2"`
	SignType     SignType        `json:"sign_type" binding:"required,oneof=SINGLE ANY ALL MAJORITY"`
	JoinStrategy string         `json:"join_string" binding:"required,oneof=ALL_PASS ANY_PASS MAJORITY_PASS"`
}

type ParallelGroupSignRequest struct {
	VendorID uint64 `json:"vendor_id" binding:"required"`
	GroupID  uint64 `json:"group_id" binding:"required"`
	SubStage ApprovalStage `json:"sub_stage" binding:"required"`
	Remark   string `json:"remark"`
}

type APIKey struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	AppKey    string     `gorm:"size:128;not null;index;uniqueIndex:idx_appkey_version,priority:1" json:"app_key"`
	AppSecret string     `gorm:"size:256;not null" json:"app_secret"`
	Status    string     `gorm:"size:16;not null;default:ACTIVE;index" json:"status"`
	Version   int        `gorm:"not null;default:1;uniqueIndex:idx_appkey_version,priority:2" json:"version"`
	ExpireAt  *time.Time `json:"expire_at,omitempty"`
	Remark    string     `gorm:"size:256" json:"remark"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type APIKeyRotateRequest struct {
	AppKey       string     `json:"app_key" binding:"required"`
	NewAppSecret string     `json:"new_app_secret"`
	OldVersion   int        `json:"old_expire_hours"`
	Remark       string     `json:"remark"`
}

