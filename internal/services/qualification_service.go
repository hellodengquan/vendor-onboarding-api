package services

import (
	"errors"
	"time"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/models"
)

type QualificationService struct{}

func NewQualificationService() *QualificationService {
	return &QualificationService{}
}

func (s *QualificationService) Upload(vendorID uint64, req *models.QualificationUploadRequest) (*models.Qualification, error) {
	db := database.GetDB()

	var vendor models.Vendor
	if err := db.First(&vendor, vendorID).Error; err != nil {
		return nil, errors.New("供应商不存在")
	}

	q := &models.Qualification{
		VendorID:   vendorID,
		Type:       req.Type,
		Name:       req.Name,
		FileURL:    req.FileURL,
		Number:     req.Number,
		IssuedBy:   req.IssuedBy,
		IssuedDate: req.IssuedDate,
		ExpiryDate: req.ExpiryDate,
		Status:     models.QualificationStatusPending,
	}

	if err := db.Create(q).Error; err != nil {
		return nil, err
	}
	return q, nil
}

func (s *QualificationService) GetByVendorID(vendorID uint64) ([]models.Qualification, error) {
	db := database.GetDB()
	var list []models.Qualification
	if err := db.Where("vendor_id = ?", vendorID).Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *QualificationService) GetByID(id uint64) (*models.Qualification, error) {
	db := database.GetDB()
	var q models.Qualification
	if err := db.First(&q, id).Error; err != nil {
		return nil, err
	}
	return &q, nil
}

func (s *QualificationService) Verify(id uint64, verifierID uint64, req *models.QualificationVerifyRequest) (*models.Qualification, error) {
	db := database.GetDB()
	q, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	if q.Status != models.QualificationStatusPending {
		return nil, errors.New("该资质已审核，不可重复审核")
	}

	now := time.Now()
	q.Status = req.Status
	q.VerifyRemark = req.VerifyRemark
	q.VerifiedBy = verifierID
	q.VerifiedAt = &now

	if err := db.Save(q).Error; err != nil {
		return nil, err
	}
	return q, nil
}

func (s *QualificationService) Delete(id uint64) error {
	db := database.GetDB()
	q, err := s.GetByID(id)
	if err != nil {
		return err
	}
	if q.Status != models.QualificationStatusPending {
		return errors.New("已审核的资质不可删除")
	}
	return db.Delete(q).Error
}
