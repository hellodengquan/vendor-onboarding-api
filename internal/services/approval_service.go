package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/models"
)

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "UNIQUE constraint failed") ||
		strings.Contains(s, "Duplicate entry") ||
		strings.Contains(s, "violates unique constraint")
}

var ErrVersionConflict = errors.New("审批版本冲突，请刷新后重试")

var StageOrderMap = map[models.ApprovalStage]int{
	models.StageDataCollection:  0,
	models.StageComplianceCheck: 1,
	models.StageLevel1Approval:  2,
	models.StageLevel2Approval:  3,
	models.StageFinanceApproval: 4,
	models.StageLegalApproval:   5,
	models.StageParallelGroup:   6,
	models.StageCompleted:       7,
	models.StageRejected:        -1,
	models.StageWithdrawn:       -2,
}

type ApprovalService struct{}

func NewApprovalService() *ApprovalService {
	return &ApprovalService{}
}

func (s *ApprovalService) CreateNodeConfig(req *models.NodeConfigCreateRequest) (*models.ApprovalNodeConfig, error) {
	db := database.GetDB()
	order, ok := StageOrderMap[req.Stage]
	if !ok {
		return nil, errors.New("无效的阶段")
	}

	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var existing models.ApprovalNodeConfig
	if err := tx.Where("stage = ?", req.Stage).First(&existing).Error; err == nil {
		if err := tx.Where("approval_node_id = ?", existing.ID).Delete(&models.ApprovalNodeSigner{}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		existing.StageName = req.StageName
		existing.SignType = req.SignType
		existing.StageOrder = order
		if err := tx.Save(&existing).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, si := range req.Signers {
			signer := &models.ApprovalNodeSigner{
				ApprovalNodeID: existing.ID,
				ApproverID:     si.ApproverID,
				ApproverName:   si.ApproverName,
				ApproverRole:   si.ApproverRole,
				Stage:          req.Stage,
			}
			if err := tx.Create(signer).Error; err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		tx.Commit()
		return &existing, nil
	}

	node := &models.ApprovalNodeConfig{
		Stage:     req.Stage,
		StageOrder: order,
		StageName: req.StageName,
		SignType:  req.SignType,
		IsEnabled: true,
	}
	if err := tx.Create(node).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	for _, si := range req.Signers {
		signer := &models.ApprovalNodeSigner{
			ApprovalNodeID: node.ID,
			ApproverID:     si.ApproverID,
			ApproverName:   si.ApproverName,
			ApproverRole:   si.ApproverRole,
			Stage:          req.Stage,
		}
		if err := tx.Create(signer).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	tx.Commit()
	return node, nil
}

func (s *ApprovalService) ListNodeConfigs() ([]models.ApprovalNodeConfig, error) {
	db := database.GetDB()
	var list []models.ApprovalNodeConfig
	if err := db.Order("stage_order ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *ApprovalService) GetNodeSigners(stage models.ApprovalStage) ([]models.ApprovalNodeSigner, error) {
	db := database.GetDB()
	var signers []models.ApprovalNodeSigner
	if err := db.Where("stage = ?", stage).Find(&signers).Error; err != nil {
		return nil, err
	}
	return signers, nil
}

func (s *ApprovalService) GetFlow(vendorID uint64) (*models.FlowDetailResponse, error) {
	db := database.GetDB()

	var vendor models.Vendor
	if err := db.First(&vendor, vendorID).Error; err != nil {
		return nil, errors.New("供应商不存在")
	}

	var flow models.ApprovalFlow
	if err := db.Where("vendor_id = ?", vendorID).First(&flow).Error; err != nil {
		return nil, errors.New("审批流程不存在")
	}

	var records []models.ApprovalRecord
	db.Where("vendor_id = ?", vendorID).Order("stage_order ASC, created_at ASC").Find(&records)

	signers, _ := s.GetNodeSigners(flow.CurrentStage)

	var nodeConfigs []models.ApprovalNodeConfig
	db.Order("stage_order ASC").Find(&nodeConfigs)

	return &models.FlowDetailResponse{
		Flow:           flow,
		Records:        records,
		Vendor:         vendor,
		CurrentSigners: signers,
		NodeConfigs:    nodeConfigs,
	}, nil
}

func (s *ApprovalService) GetStageSignStatus(vendorID uint64, stage models.ApprovalStage) (*models.StageSignStatus, error) {
	db := database.GetDB()

	var nodeConfig models.ApprovalNodeConfig
	if err := db.Where("stage = ?", stage).First(&nodeConfig).Error; err != nil {
		return nil, errors.New("审批节点未配置")
	}

	signers, err := s.GetNodeSigners(stage)
	if err != nil {
		return nil, err
	}

	var records []models.ApprovalRecord
	db.Where("vendor_id = ? AND stage = ?", vendorID, stage).Find(&records)

	approvedCount := 0
	rejectedCount := 0
	for _, r := range records {
		if r.Status == models.ApprovalStatusApproved {
			approvedCount++
		} else if r.Status == models.ApprovalStatusRejected {
			rejectedCount++
		}
	}

	isComplete := s.evaluateSignCondition(nodeConfig.SignType, len(signers), approvedCount, rejectedCount)

	return &models.StageSignStatus{
		Stage:         stage,
		StageName:     nodeConfig.StageName,
		SignType:      nodeConfig.SignType,
		TotalSigners:  len(signers),
		ApprovedCount: approvedCount,
		RejectedCount: rejectedCount,
		IsComplete:    isComplete,
		Signers:       signers,
		Records:       records,
	}, nil
}

func (s *ApprovalService) evaluateSignCondition(signType models.SignType, totalSigners int, approvedCount int, rejectedCount int) bool {
	if totalSigners == 0 {
		return true
	}
	switch signType {
	case models.SignTypeSingle:
		return approvedCount >= 1 || rejectedCount >= 1
	case models.SignTypeAny:
		return approvedCount >= 1 || rejectedCount >= 1
	case models.SignTypeAll:
		return approvedCount+rejectedCount >= totalSigners
	case models.SignTypeMajority:
		threshold := totalSigners/2 + 1
		if totalSigners%2 == 0 {
			threshold = totalSigners/2 + 1
		}
		return approvedCount >= threshold || rejectedCount >= threshold
	default:
		return approvedCount >= 1
	}
}

func (s *ApprovalService) Approve(req *models.ApprovalRequest, approverID uint64, approverName string) error {
	ctx := context.Background()

	use2PC := database.Registry != nil && database.Registry.TPM() != nil

	var mainTx, auditTx *gorm.DB
	var commitFn func() error
	var rollbackFn func() error

	if use2PC {
		tpm := database.Registry.TPM()
		txs, err := tpm.Begin(ctx)
		if err != nil {
			return err
		}
		mainTx = txs["main"]
		auditTx = txs["audit"]
		commitFn = func() error {
			if err := tpm.Prepare(); err != nil {
				return err
			}
			return tpm.Commit()
		}
		rollbackFn = func() error {
			return tpm.Rollback()
		}
	} else {
		db := database.GetDB()
		mainTx = db.Begin()
		if mainTx.Error != nil {
			return mainTx.Error
		}
		auditTx = mainTx
		commitFn = func() error {
			return mainTx.Commit().Error
		}
		rollbackFn = func() error {
			return mainTx.Rollback().Error
		}
	}

	defer func() {
		if r := recover(); r != nil {
			_ = rollbackFn()
			panic(r)
		}
	}()

	var flow models.ApprovalFlow
	if err := mainTx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("vendor_id = ?", req.VendorID).First(&flow).Error; err != nil {
		_ = rollbackFn()
		return errors.New("审批流程不存在")
	}
	expectedVersion := flow.Version
	beforeStatus := string(flow.Status)
	beforeStage := string(flow.CurrentStage)

	if flow.Status == models.ApprovalStatusApproved {
		_ = rollbackFn()
		return errors.New("流程已完成审批")
	}
	if flow.Status == models.ApprovalStatusRejected {
		_ = rollbackFn()
		return errors.New("流程已被驳回")
	}

	if err := s.validateSignerPermission(flow.CurrentStage, approverID); err != nil {
		_ = rollbackFn()
		return err
	}

	var existing models.ApprovalRecord
	if err := mainTx.Where("vendor_id = ? AND stage = ? AND approver_id = ?",
		req.VendorID, flow.CurrentStage, approverID).First(&existing).Error; err == nil {
		_ = rollbackFn()
		return errors.New("该审批人已在本阶段签署过")
	}

	now := time.Now()
	record := &models.ApprovalRecord{
		VendorID:     req.VendorID,
		Stage:        flow.CurrentStage,
		StageOrder:   flow.StageOrder,
		Status:       models.ApprovalStatusApproved,
		ApproverID:   approverID,
		ApproverName: approverName,
		Remark:       req.Remark,
		ApprovedAt:   &now,
	}
	if err := mainTx.Create(record).Error; err != nil {
		_ = rollbackFn()
		if isUniqueViolation(err) {
			return errors.New("该审批人已在本阶段签署过（并发冲突）")
		}
		return err
	}

	var nodeConfig models.ApprovalNodeConfig
	mainTx.Where("stage = ?", flow.CurrentStage).First(&nodeConfig)
	var approvedCount, rejectedCount int64
	mainTx.Model(&models.ApprovalRecord{}).
		Where("vendor_id = ? AND stage = ? AND status = ?", req.VendorID, flow.CurrentStage, models.ApprovalStatusApproved).
		Count(&approvedCount)
	mainTx.Model(&models.ApprovalRecord{}).
		Where("vendor_id = ? AND stage = ? AND status = ?", req.VendorID, flow.CurrentStage, models.ApprovalStatusRejected).
		Count(&rejectedCount)

	flow.ApprovedCount = int(approvedCount)
	flow.RejectedCount = int(rejectedCount)
	signerCount, _ := s.countSigners(flow.CurrentStage)
	flow.SignerCount = signerCount
	flow.Version++

	if rejectedCount > 0 {
		if err := s.rejectFlow(mainTx, &flow, req.VendorID, approverID, approverName, "会签中有人驳回，流程终止"); err != nil {
			_ = rollbackFn()
			return err
		}
		result := mainTx.Model(&flow).Where("id = ? AND version = ?", flow.ID, expectedVersion).Updates(map[string]interface{}{
			"status":         flow.Status,
			"approved_count": flow.ApprovedCount,
			"rejected_count": flow.RejectedCount,
			"signer_count":   flow.SignerCount,
			"version":        flow.Version,
		})
		if result.Error != nil {
			_ = rollbackFn()
			return result.Error
		}
		if result.RowsAffected == 0 {
			_ = rollbackFn()
			return ErrVersionConflict
		}
		if auditTx != nil {
			meta, _ := json.Marshal(map[string]interface{}{
				"approver_id":   approverID,
				"approver_name": approverName,
				"remark":        req.Remark,
				"reject_reason": "会签中有人驳回",
			})
			_ = auditTx.Create(&models.AuditLog{
				VendorID:     req.VendorID,
				Action:       models.AuditActionReject,
				ActorID:      approverID,
				ActorName:    approverName,
				BeforeStatus: beforeStatus,
				AfterStatus:  string(flow.Status),
				BeforeStage:  beforeStage,
				AfterStage:   string(flow.CurrentStage),
				Metadata:     string(meta),
				CreatedAt:    now,
			}).Error
		}
		return commitFn()
	}

	isComplete := s.evaluateSignCondition(nodeConfig.SignType, signerCount, int(approvedCount), int(rejectedCount))
	if isComplete {
		if err := s.advanceToNextStage(mainTx, &flow, req.VendorID); err != nil {
			_ = rollbackFn()
			return err
		}
	} else if approvedCount > 0 && approvedCount < int64(signerCount) {
		flow.Status = models.ApprovalStatusPartial
	}

	result := mainTx.Model(&flow).Where("id = ? AND version = ?", flow.ID, expectedVersion).Updates(map[string]interface{}{
		"current_stage":  flow.CurrentStage,
		"stage_order":    flow.StageOrder,
		"status":         flow.Status,
		"approved_count": flow.ApprovedCount,
		"rejected_count": flow.RejectedCount,
		"signer_count":   flow.SignerCount,
		"version":        flow.Version,
	})
	if result.Error != nil {
		_ = rollbackFn()
		return result.Error
	}
	if result.RowsAffected == 0 {
		_ = rollbackFn()
		return ErrVersionConflict
	}

	if auditTx != nil {
		meta, _ := json.Marshal(map[string]interface{}{
			"approver_id":      approverID,
			"approver_name":    approverName,
			"remark":           req.Remark,
			"approved_count":   approvedCount,
			"total_signers":    signerCount,
			"stage_complete":   isComplete,
			"flow_version":     flow.Version,
		})
		_ = auditTx.Create(&models.AuditLog{
			VendorID:     req.VendorID,
			Action:       models.AuditActionApprove,
			ActorID:      approverID,
			ActorName:    approverName,
			BeforeStatus: beforeStatus,
			AfterStatus:  string(flow.Status),
			BeforeStage:  beforeStage,
			AfterStage:   string(flow.CurrentStage),
			Metadata:     string(meta),
			CreatedAt:    now,
		}).Error
	}

	return commitFn()
}

func (s *ApprovalService) Reject(req *models.ApprovalRejectRequest, approverID uint64, approverName string) error {
	db := database.GetDB()

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var flow models.ApprovalFlow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("vendor_id = ?", req.VendorID).First(&flow).Error; err != nil {
		tx.Rollback()
		return errors.New("审批流程不存在")
	}
	expectedVersion := flow.Version

	if flow.Status == models.ApprovalStatusApproved {
		tx.Rollback()
		return errors.New("流程已完成审批，不可驳回")
	}
	if flow.Status == models.ApprovalStatusRejected {
		tx.Rollback()
		return errors.New("流程已被驳回")
	}

	if err := s.validateSignerPermission(flow.CurrentStage, approverID); err != nil {
		tx.Rollback()
		return err
	}

	now := time.Now()
	record := &models.ApprovalRecord{
		VendorID:     req.VendorID,
		Stage:        flow.CurrentStage,
		StageOrder:   flow.StageOrder,
		Status:       models.ApprovalStatusRejected,
		ApproverID:   approverID,
		ApproverName: approverName,
		Remark:       req.Remark,
		ApprovedAt:   &now,
	}
	if err := tx.Create(record).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := s.rejectFlow(tx, &flow, req.VendorID, approverID, approverName, req.Remark); err != nil {
		tx.Rollback()
		return err
	}
	flow.Version++

	result := tx.Model(&flow).Where("id = ? AND version = ?", flow.ID, expectedVersion).Updates(map[string]interface{}{
		"current_stage":  flow.CurrentStage,
		"stage_order":    flow.StageOrder,
		"status":         flow.Status,
		"approved_count": flow.ApprovedCount,
		"rejected_count": flow.RejectedCount,
		"signer_count":   flow.SignerCount,
		"version":        flow.Version,
	})
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		return errors.New("审批版本冲突，请刷新后重试")
	}
	return tx.Commit().Error
}

func (s *ApprovalService) rejectFlow(db *gorm.DB, flow *models.ApprovalFlow, vendorID uint64, approverID uint64, approverName string, remark string) error {
	flow.CurrentStage = models.StageRejected
	flow.StageOrder = StageOrderMap[models.StageRejected]
	flow.Status = models.ApprovalStatusRejected
	flow.RejectedCount++

	var vendor models.Vendor
	db.First(&vendor, vendorID)
	vendor.Status = models.VendorStatusRejected
	return db.Save(&vendor).Error
}

func (s *ApprovalService) advanceToNextStage(db *gorm.DB, flow *models.ApprovalFlow, vendorID uint64) error {
	nextStage, nextOrder := s.getNextStage(flow.CurrentStage)
	if nextStage == models.StageCompleted {
		flow.CurrentStage = models.StageCompleted
		flow.StageOrder = StageOrderMap[models.StageCompleted]
		flow.Status = models.ApprovalStatusApproved
		flow.ApprovedCount = 0
		flow.RejectedCount = 0
		flow.SignerCount = 0
		var vendor models.Vendor
		db.First(&vendor, vendorID)
		vendor.Status = models.VendorStatusApproved
		return db.Save(&vendor).Error
	}

	flow.CurrentStage = nextStage
	flow.StageOrder = nextOrder
	flow.Status = models.ApprovalStatusPending
	flow.ApprovedCount = 0
	flow.RejectedCount = 0
	signerCount, _ := s.countSigners(nextStage)
	flow.SignerCount = signerCount

	var vendor models.Vendor
	db.First(&vendor, vendorID)
	switch nextStage {
	case models.StageLevel1Approval, models.StageLevel2Approval:
		vendor.Status = models.VendorStatusPendingApproval
	case models.StageComplianceCheck:
		vendor.Status = models.VendorStatusComplianceCheck
	}
	return db.Save(&vendor).Error
}

func (s *ApprovalService) Transition(req *models.StageTransitionRequest, operatorID uint64, operatorName string) error {
	db := database.GetDB()

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var flow models.ApprovalFlow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("vendor_id = ?", req.VendorID).First(&flow).Error; err != nil {
		tx.Rollback()
		return errors.New("审批流程不存在")
	}
	expectedVersion := flow.Version

	toOrder, exists := StageOrderMap[req.ToStage]
	if !exists {
		tx.Rollback()
		return errors.New("目标阶段无效")
	}

	if toOrder <= flow.StageOrder && req.ToStage != models.StageRejected {
		tx.Rollback()
		return errors.New("不能回退到之前的阶段")
	}

	now := time.Now()
	record := &models.ApprovalRecord{
		VendorID:     req.VendorID,
		Stage:        req.ToStage,
		StageOrder:   toOrder,
		Status:       models.ApprovalStatusApproved,
		ApproverID:   operatorID,
		ApproverName: operatorName,
		Remark:       fmt.Sprintf("手动流转: %s", req.Remark),
		ApprovedAt:   &now,
	}
	if err := tx.Create(record).Error; err != nil {
		tx.Rollback()
		return err
	}

	flow.CurrentStage = req.ToStage
	flow.StageOrder = toOrder
	flow.ApprovedCount = 0
	flow.RejectedCount = 0
	if req.ToStage == models.StageCompleted {
		flow.Status = models.ApprovalStatusApproved
		var vendor models.Vendor
		tx.First(&vendor, req.VendorID)
		vendor.Status = models.VendorStatusApproved
		tx.Save(&vendor)
	} else if req.ToStage == models.StageRejected {
		flow.Status = models.ApprovalStatusRejected
		var vendor models.Vendor
		tx.First(&vendor, req.VendorID)
		vendor.Status = models.VendorStatusRejected
		tx.Save(&vendor)
	}
	signerCount, _ := s.countSigners(req.ToStage)
	flow.SignerCount = signerCount
	flow.Version++

	result := tx.Model(&flow).Where("id = ? AND version = ?", flow.ID, expectedVersion).Updates(map[string]interface{}{
		"current_stage":  flow.CurrentStage,
		"stage_order":    flow.StageOrder,
		"status":         flow.Status,
		"approved_count": flow.ApprovedCount,
		"rejected_count": flow.RejectedCount,
		"signer_count":   flow.SignerCount,
		"version":        flow.Version,
	})
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		return errors.New("审批版本冲突，请刷新后重试")
	}
	return tx.Commit().Error
}

func (s *ApprovalService) validateSignerPermission(stage models.ApprovalStage, approverID uint64) error {
	signers, err := s.GetNodeSigners(stage)
	if err != nil {
		return err
	}
	if len(signers) == 0 {
		return nil
	}
	for _, sg := range signers {
		if sg.ApproverID == approverID {
			return nil
		}
	}
	return errors.New("当前审批人不在该节点会签名单内")
}

func (s *ApprovalService) countSigners(stage models.ApprovalStage) (int, error) {
	db := database.GetDB()
	var count int64
	if err := db.Model(&models.ApprovalNodeSigner{}).Where("stage = ?", stage).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *ApprovalService) getNextStage(current models.ApprovalStage) (models.ApprovalStage, int) {
	switch current {
	case models.StageDataCollection:
		return models.StageComplianceCheck, StageOrderMap[models.StageComplianceCheck]
	case models.StageComplianceCheck:
		return models.StageLevel1Approval, StageOrderMap[models.StageLevel1Approval]
	case models.StageLevel1Approval:
		return models.StageLevel2Approval, StageOrderMap[models.StageLevel2Approval]
	case models.StageLevel2Approval:
		return models.StageFinanceApproval, StageOrderMap[models.StageFinanceApproval]
	case models.StageFinanceApproval:
		return models.StageLegalApproval, StageOrderMap[models.StageLegalApproval]
	case models.StageLegalApproval:
		return models.StageCompleted, StageOrderMap[models.StageCompleted]
	default:
		return models.StageCompleted, StageOrderMap[models.StageCompleted]
	}
}

func (s *ApprovalService) AddSigner(req *models.AddSignerRequest, addedBy uint64, addedByName string) error {
	db := database.GetDB()

	var flow models.ApprovalFlow
	if err := db.Where("vendor_id = ?", req.VendorID).First(&flow).Error; err != nil {
		return errors.New("流程不存在")
	}
	if flow.Status == models.ApprovalStatusApproved || flow.Status == models.ApprovalStatusRejected || flow.Status == models.ApprovalStatusWithdrawn {
		return errors.New("当前流程状态不允许加签")
	}

	stage := req.Stage
	if stage == "" {
		stage = flow.CurrentStage
	}

	var maxSeq int64
	db.Model(&models.ApprovalNodeSigner{}).Where("stage = ?", stage).Select("COALESCE(MAX(sequence),0)").Scan(&maxSeq)
	var maxAdd int64
	db.Model(&models.AdditionalSignerRecord{}).Where("vendor_id = ? AND stage = ?", req.VendorID, stage).Select("COALESCE(MAX(sequence),0)").Scan(&maxAdd)
	seq := int(maxSeq) + int(maxAdd) + 1 + req.Sequence

	var nodeCfg models.ApprovalNodeConfig
	db.Where("stage = ?", stage).First(&nodeCfg)
	if nodeCfg.ID > 0 {
		signer := &models.ApprovalNodeSigner{
			ApprovalNodeID: nodeCfg.ID,
			ApproverID:     req.ApproverID,
			ApproverName:   req.ApproverName,
			ApproverRole:   req.ApproverRole,
			Stage:          stage,
			Source:         models.SignerSourceAddition,
			Sequence:       seq,
			AddedBy:        &addedBy,
		}
		if err := db.Create(signer).Error; err != nil {
			return err
		}
	}

	add := &models.AdditionalSignerRecord{
		VendorID:     req.VendorID,
		Stage:        stage,
		ApproverID:   req.ApproverID,
		ApproverName: req.ApproverName,
		ApproverRole: req.ApproverRole,
		AddMode:      req.AddMode,
		AddedBy:      addedBy,
		AddedByName:  addedByName,
		Remark:       req.Remark,
		Sequence:     seq,
	}
	if err := db.Create(add).Error; err != nil {
		return err
	}

	signerCount, _ := s.countSigners(stage)
	if stage == flow.CurrentStage {
		flow.SignerCount = signerCount
		db.Save(&flow)
	}
	return nil
}

func (s *ApprovalService) Withdraw(req *models.WithdrawRequest, withdrawnBy uint64, withdrawnByName string) error {
	db := database.GetDB()

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var flow models.ApprovalFlow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("vendor_id = ?", req.VendorID).First(&flow).Error; err != nil {
		tx.Rollback()
		return errors.New("流程不存在")
	}
	expectedVersion := flow.Version

	if flow.Status == models.ApprovalStatusApproved {
		tx.Rollback()
		return errors.New("已完成的审批不可撤回")
	}

	toStage := req.ToStage
	toOrder := 0
	if toStage != "" {
		var ok bool
		toOrder, ok = StageOrderMap[toStage]
		if !ok {
			tx.Rollback()
			return errors.New("目标阶段无效")
		}
		if toOrder >= flow.StageOrder {
			tx.Rollback()
			return errors.New("只能回退到更早的阶段")
		}
	} else {
		switch flow.CurrentStage {
		case models.StageDataCollection:
			tx.Rollback()
			return errors.New("当前为初始阶段，无需撤回")
		case models.StageComplianceCheck:
			toStage = models.StageDataCollection
		case models.StageLevel1Approval:
			toStage = models.StageComplianceCheck
		case models.StageLevel2Approval:
			toStage = models.StageLevel1Approval
		case models.StageFinanceApproval:
			toStage = models.StageLevel2Approval
		case models.StageLegalApproval:
			toStage = models.StageFinanceApproval
		default:
			toStage = models.StageLevel1Approval
		}
		toOrder = StageOrderMap[toStage]
	}

	now := time.Now()
	record := &models.WithdrawalRecord{
		VendorID:        req.VendorID,
		FromStage:       flow.CurrentStage,
		FromStageOrder:  flow.StageOrder,
		ToStage:         toStage,
		ToStageOrder:    toOrder,
		WithdrawnBy:     withdrawnBy,
		WithdrawnByName: withdrawnByName,
		Reason:          req.Reason,
		IsRollback:      req.IsRollback,
		CreatedAt:       now,
	}
	if err := tx.Create(record).Error; err != nil {
		tx.Rollback()
		return err
	}

	flow.CurrentStage = toStage
	flow.StageOrder = toOrder
	flow.Status = models.ApprovalStatusWithdrawn
	flow.ApprovedCount = 0
	flow.RejectedCount = 0
	flow.SignerCount = 0
	flow.Version++
	flow.WithdrawnBy = &withdrawnBy
	flow.WithdrawnAt = &now

	result := tx.Model(&flow).Where("id = ? AND version = ?", flow.ID, expectedVersion).Updates(map[string]interface{}{
		"current_stage":  flow.CurrentStage,
		"stage_order":    flow.StageOrder,
		"status":         flow.Status,
		"approved_count": flow.ApprovedCount,
		"rejected_count": flow.RejectedCount,
		"signer_count":   flow.SignerCount,
		"version":        flow.Version,
		"withdrawn_by":   flow.WithdrawnBy,
		"withdrawn_at":   flow.WithdrawnAt,
	})
	if result.Error != nil {
		tx.Rollback()
		return result.Error
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		return errors.New("审批版本冲突，请刷新后重试")
	}

	if req.IsRollback {
		var vendor models.Vendor
		tx.First(&vendor, req.VendorID)
		switch toStage {
		case models.StageDataCollection:
			vendor.Status = models.VendorStatusDraft
		case models.StageComplianceCheck:
			vendor.Status = models.VendorStatusComplianceCheck
		default:
			vendor.Status = models.VendorStatusPendingApproval
		}
		if err := tx.Save(&vendor).Error; err != nil {
			tx.Rollback()
			return err
		}

		go func(vid uint64) {
			time.Sleep(100 * time.Millisecond)
			db.Model(&models.ApprovalFlow{}).Where("vendor_id = ?", vid).Update("status", models.ApprovalStatusPending)
		}(req.VendorID)
	} else {
		var vendor models.Vendor
		tx.First(&vendor, req.VendorID)
		vendor.Status = models.VendorStatusDraft
		if err := tx.Save(&vendor).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

func (s *ApprovalService) CreateParallelGroup(req *models.ParallelGroupCreateRequest) (*models.ParallelGroup, error) {
	db := database.GetDB()
	if len(req.SubStages) < 2 {
		return nil, errors.New("并签组至少需要两个子阶段")
	}
	subStagesJSON := "["
	for i, ss := range req.SubStages {
		if i > 0 {
			subStagesJSON += ","
		}
		subStagesJSON += "\"" + string(ss) + "\""
	}
	subStagesJSON += "]"

	group := &models.ParallelGroup{
		GroupCode:    req.GroupCode,
		Name:         req.Name,
		SubStages:    subStagesJSON,
		SignType:     req.SignType,
		JoinStrategy: req.JoinStrategy,
		IsEnabled:    true,
	}
	if err := db.Create(group).Error; err != nil {
		return nil, err
	}
	return group, nil
}

func (s *ApprovalService) ListWithdrawals(vendorID uint64) ([]models.WithdrawalRecord, error) {
	db := database.GetDB()
	var list []models.WithdrawalRecord
	if err := db.Where("vendor_id = ?", vendorID).Order("id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
