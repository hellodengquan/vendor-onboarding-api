package services

import (
	"errors"
	"time"

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

	return &models.FlowDetailResponse{
		Flow:    flow,
		Records: records,
		Vendor:  vendor,
	}, nil
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

	nextStage, nextOrder := s.getNextStage(flow.CurrentStage)
	if nextStage == models.StageCompleted {
		flow.CurrentStage = models.StageCompleted
		flow.StageOrder = StageOrderMap[models.StageCompleted]
		flow.Status = models.ApprovalStatusApproved
		db.Save(&flow)

		var vendor models.Vendor
		db.First(&vendor, req.VendorID)
		vendor.Status = models.VendorStatusApproved
		db.Save(&vendor)
	} else {
		flow.CurrentStage = nextStage
		flow.StageOrder = nextOrder
		db.Save(&flow)

		var vendor models.Vendor
		db.First(&vendor, req.VendorID)
		switch nextStage {
		case models.StageLevel1Approval:
			vendor.Status = models.VendorStatusPendingApproval
		case models.StageLevel2Approval:
			vendor.Status = models.VendorStatusPendingApproval
		}
		db.Save(&vendor)
	}

	return nil
}

func (s *ApprovalService) Reject(req *models.ApprovalRejectRequest, approverID uint64, approverName string) error {
	db := database.GetDB()

	var flow models.ApprovalFlow
	if err := db.Where("vendor_id = ?", req.VendorID).First(&flow).Error; err != nil {
		return errors.New("审批流程不存在")
	}

	if flow.Status != models.ApprovalStatusPending {
		return errors.New("当前状态不可驳回")
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

	flow.CurrentStage = models.StageRejected
	flow.StageOrder = StageOrderMap[models.StageRejected]
	flow.Status = models.ApprovalStatusRejected
	db.Save(&flow)

	var vendor models.Vendor
	db.First(&vendor, req.VendorID)
	vendor.Status = models.VendorStatusRejected
	db.Save(&vendor)

	return nil
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
		Remark:       req.Remark,
		ApprovedAt:   &now,
	}
	if err := db.Create(record).Error; err != nil {
		return err
	}

	flow.CurrentStage = req.ToStage
	flow.StageOrder = toOrder
	if req.ToStage == models.StageCompleted {
		flow.Status = models.ApprovalStatusApproved
	} else if req.ToStage == models.StageRejected {
		flow.Status = models.ApprovalStatusRejected
	}
	db.Save(&flow)

	return nil
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
