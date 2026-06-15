package services

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/models"
)

var StageOrderMap = map[models.ApprovalStage]int{
	models.StageDataCollection:  0,
	models.StageComplianceCheck: 1,
	models.StageLevel1Approval:  2,
	models.StageLevel2Approval:  3,
	models.StageCompleted:       4,
	models.StageRejected:        -1,
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
	db := database.GetDB()

	var flow models.ApprovalFlow
	if err := db.Where("vendor_id = ?", req.VendorID).First(&flow).Error; err != nil {
		return errors.New("审批流程不存在")
	}

	if flow.Status == models.ApprovalStatusApproved {
		return errors.New("流程已完成审批")
	}
	if flow.Status == models.ApprovalStatusRejected {
		return errors.New("流程已被驳回")
	}

	if err := s.validateSignerPermission(flow.CurrentStage, approverID); err != nil {
		return err
	}

	var existing models.ApprovalRecord
	if err := db.Where("vendor_id = ? AND stage = ? AND approver_id = ?",
		req.VendorID, flow.CurrentStage, approverID).First(&existing).Error; err == nil {
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
	if err := db.Create(record).Error; err != nil {
		return err
	}

	flow.ApprovedCount++
	signerCount, _ := s.countSigners(flow.CurrentStage)
	flow.SignerCount = signerCount

	stageStatus, _ := s.GetStageSignStatus(req.VendorID, flow.CurrentStage)
	if flow.RejectedCount > 0 {
		return s.rejectFlow(db, &flow, req.VendorID, approverID, approverName, "会签中有人驳回，流程终止")
	}

	if stageStatus != nil && stageStatus.IsComplete {
		return s.advanceToNextStage(db, &flow, req.VendorID)
	}

	if flow.ApprovedCount > 0 && flow.ApprovedCount < signerCount {
		flow.Status = models.ApprovalStatusPartial
	}
	return db.Save(&flow).Error
}

func (s *ApprovalService) Reject(req *models.ApprovalRejectRequest, approverID uint64, approverName string) error {
	db := database.GetDB()

	var flow models.ApprovalFlow
	if err := db.Where("vendor_id = ?", req.VendorID).First(&flow).Error; err != nil {
		return errors.New("审批流程不存在")
	}

	if flow.Status == models.ApprovalStatusApproved {
		return errors.New("流程已完成审批，不可驳回")
	}
	if flow.Status == models.ApprovalStatusRejected {
		return errors.New("流程已被驳回")
	}

	if err := s.validateSignerPermission(flow.CurrentStage, approverID); err != nil {
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
	if err := db.Create(record).Error; err != nil {
		return err
	}

	return s.rejectFlow(db, &flow, req.VendorID, approverID, approverName, req.Remark)
}

func (s *ApprovalService) rejectFlow(db *gorm.DB, flow *models.ApprovalFlow, vendorID uint64, approverID uint64, approverName string, remark string) error {
	flow.CurrentStage = models.StageRejected
	flow.StageOrder = StageOrderMap[models.StageRejected]
	flow.Status = models.ApprovalStatusRejected
	flow.RejectedCount++
	if err := db.Save(flow).Error; err != nil {
		return err
	}

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
		if err := db.Save(flow).Error; err != nil {
			return err
		}
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
	if err := db.Save(flow).Error; err != nil {
		return err
	}

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

	var flow models.ApprovalFlow
	if err := db.Where("vendor_id = ?", req.VendorID).First(&flow).Error; err != nil {
		return errors.New("审批流程不存在")
	}

	toOrder, exists := StageOrderMap[req.ToStage]
	if !exists {
		return errors.New("目标阶段无效")
	}

	if toOrder <= flow.StageOrder && req.ToStage != models.StageRejected {
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
	if err := db.Create(record).Error; err != nil {
		return err
	}

	flow.CurrentStage = req.ToStage
	flow.StageOrder = toOrder
	flow.ApprovedCount = 0
	flow.RejectedCount = 0
	if req.ToStage == models.StageCompleted {
		flow.Status = models.ApprovalStatusApproved
		var vendor models.Vendor
		db.First(&vendor, req.VendorID)
		vendor.Status = models.VendorStatusApproved
		db.Save(&vendor)
	} else if req.ToStage == models.StageRejected {
		flow.Status = models.ApprovalStatusRejected
		var vendor models.Vendor
		db.First(&vendor, req.VendorID)
		vendor.Status = models.VendorStatusRejected
		db.Save(&vendor)
	}
	signerCount, _ := s.countSigners(req.ToStage)
	flow.SignerCount = signerCount
	return db.Save(&flow).Error
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
		return models.StageCompleted, StageOrderMap[models.StageCompleted]
	default:
		return models.StageCompleted, StageOrderMap[models.StageCompleted]
	}
}
