package services

import (
	"errors"
	"time"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/pkg/utils"
)

type VendorService struct{}

func NewVendorService() *VendorService {
	return &VendorService{}
}

func (s *VendorService) Create(req *models.VendorCreateRequest) (*models.Vendor, error) {
	db := database.GetDB()

	var existing models.Vendor
	if req.UnifiedSocialCreditCode != "" {
		result := db.Where("unified_social_credit_code = ?", req.UnifiedSocialCreditCode).First(&existing)
		if result.Error == nil {
			return nil, errors.New("统一社会信用代码已存在")
		}
	}

	vendor := &models.Vendor{
		VendorCode:            utils.GenerateVendorCode(),
		CompanyName:           req.CompanyName,
		UnifiedSocialCreditCode: req.UnifiedSocialCreditCode,
		LegalPerson:           req.LegalPerson,
		ContactPerson:         req.ContactPerson,
		ContactPhone:          req.ContactPhone,
		ContactEmail:          req.ContactEmail,
		RegisteredAddress:     req.RegisteredAddress,
		BusinessScope:         req.BusinessScope,
		Status:                models.VendorStatusDraft,
	}

	if err := db.Create(vendor).Error; err != nil {
		return nil, err
	}

	flow := &models.ApprovalFlow{
		VendorID:     vendor.ID,
		CurrentStage: models.StageDataCollection,
		StageOrder:   0,
		Status:       models.ApprovalStatusPending,
	}
	if err := db.Create(flow).Error; err != nil {
		return nil, err
	}

	vendor.CurrentStageID = flow.ID
	db.Save(vendor)

	return vendor, nil
}

func (s *VendorService) GetByID(id uint64) (*models.Vendor, error) {
	db := database.GetDB()
	var vendor models.Vendor
	if err := db.First(&vendor, id).Error; err != nil {
		return nil, err
	}
	return &vendor, nil
}

func (s *VendorService) List(req *models.VendorListRequest) (*models.VendorListResponse, error) {
	db := database.GetDB()
	query := db.Model(&models.Vendor{})

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}
	if req.Keyword != "" {
		query = query.Where("company_name LIKE ? OR vendor_code LIKE ? OR contact_person LIKE ?",
			"%"+req.Keyword+"%", "%"+req.Keyword+"%", "%"+req.Keyword+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (req.Page - 1) * req.PageSize
	var list []models.Vendor
	if err := query.Order("created_at DESC").Offset(offset).Limit(req.PageSize).Find(&list).Error; err != nil {
		return nil, err
	}

	return &models.VendorListResponse{
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
		List:     list,
	}, nil
}

func (s *VendorService) Update(id uint64, req *models.VendorUpdateRequest) (*models.Vendor, error) {
	db := database.GetDB()
	vendor, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	if vendor.Status != models.VendorStatusDraft && vendor.Status != models.VendorStatusRejected {
		return nil, errors.New("当前状态不允许编辑")
	}

	if req.CompanyName != "" {
		vendor.CompanyName = req.CompanyName
	}
	if req.UnifiedSocialCreditCode != "" {
		vendor.UnifiedSocialCreditCode = req.UnifiedSocialCreditCode
	}
	if req.LegalPerson != "" {
		vendor.LegalPerson = req.LegalPerson
	}
	if req.ContactPerson != "" {
		vendor.ContactPerson = req.ContactPerson
	}
	if req.ContactPhone != "" {
		vendor.ContactPhone = req.ContactPhone
	}
	if req.ContactEmail != "" {
		vendor.ContactEmail = req.ContactEmail
	}
	if req.RegisteredAddress != "" {
		vendor.RegisteredAddress = req.RegisteredAddress
	}
	if req.BusinessScope != "" {
		vendor.BusinessScope = req.BusinessScope
	}

	if err := db.Save(vendor).Error; err != nil {
		return nil, err
	}
	return vendor, nil
}

func (s *VendorService) Submit(id uint64, submitterID uint64) error {
	db := database.GetDB()
	vendor, err := s.GetByID(id)
	if err != nil {
		return err
	}

	if vendor.Status != models.VendorStatusDraft && vendor.Status != models.VendorStatusRejected {
		return errors.New("当前状态不允许提交")
	}

	now := time.Now()
	vendor.Status = models.VendorStatusPendingReview
	vendor.SubmittedBy = submitterID
	vendor.SubmittedAt = &now

	if err := db.Save(vendor).Error; err != nil {
		return err
	}

	var flow models.ApprovalFlow
	db.Where("vendor_id = ?", id).First(&flow)
	flow.CurrentStage = models.StageComplianceCheck
	flow.StageOrder = 1
	db.Save(&flow)

	record := &models.ApprovalRecord{
		VendorID:   id,
		Stage:      models.StageDataCollection,
		StageOrder: 0,
		Status:     models.ApprovalStatusApproved,
		Remark:     "资料提交完成",
		ApprovedAt: &now,
	}
	db.Create(record)

	return nil
}
