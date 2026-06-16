package models

import "time"

type AuditAction string

const (
	AuditActionApprove        AuditAction = "APPROVE"
	AuditActionReject         AuditAction = "REJECT"
	AuditActionWithdraw       AuditAction = "WITHDRAW"
	AuditActionTransition     AuditAction = "TRANSITION"
	AuditActionAddSigner      AuditAction = "ADD_SIGNER"
	AuditActionSubmit         AuditAction = "SUBMIT"
	AuditActionCreateVendor   AuditAction = "CREATE_VENDOR"
	AuditActionUpdateVendor   AuditAction = "UPDATE_VENDOR"
	AuditActionUploadQual     AuditAction = "UPLOAD_QUAL"
	AuditActionVerifyQual     AuditAction = "VERIFY_QUAL"
)

type AuditLog struct {
	ID           uint64      `gorm:"primaryKey;autoIncrement" json:"id"`
	TraceID      string      `gorm:"size:64;index" json:"trace_id"`
	VendorID     uint64      `gorm:"index" json:"vendor_id"`
	Action       AuditAction `gorm:"size:32;not null;index" json:"action"`
	ActorID      uint64      `gorm:"index" json:"actor_id"`
	ActorName    string      `gorm:"size:64" json:"actor_name"`
	BeforeStatus string      `gorm:"size:32" json:"before_status"`
	AfterStatus  string      `gorm:"size:32" json:"after_status"`
	BeforeStage  string      `gorm:"size:32" json:"before_stage"`
	AfterStage   string      `gorm:"size:32" json:"after_stage"`
	Metadata     string      `gorm:"type:text" json:"metadata"`
	CreatedAt    time.Time   `json:"created_at"`
}

type AuditLogWithVersion struct {
	AuditLog
	FlowVersion int `json:"flow_version"`
}
