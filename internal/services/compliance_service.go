package services

import (
	"errors"
	"regexp"
	"time"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/models"
)

type ComplianceService struct{}

func NewComplianceService() *ComplianceService {
	return &ComplianceService{}
}

func (s *ComplianceService) Check(vendorID uint64) (*models.ComplianceCheckResult, error) {
	db := database.GetDB()

	var vendor models.Vendor
	if err := db.First(&vendor, vendorID).Error; err != nil {
		return nil, errors.New("供应商不存在")
	}

	var qualifications []models.Qualification
	db.Where("vendor_id = ?", vendorID).Find(&qualifications)

	result := &models.ComplianceCheckResult{
		VendorID:    vendorID,
		IsCompliant: true,
		PassedItems: []string{},
		FailedItems: []string{},
		WarningItems: []string{},
		CheckTime:   time.Now(),
	}

	s.checkBasicInfo(&vendor, result)
	s.checkQualifications(qualifications, result)

	if len(result.FailedItems) > 0 {
		result.IsCompliant = false
	}

	return result, nil
}

func (s *ComplianceService) checkBasicInfo(vendor *models.Vendor, result *models.ComplianceCheckResult) {
	if vendor.CompanyName == "" {
		result.FailedItems = append(result.FailedItems, "公司名称不能为空")
	} else {
		result.PassedItems = append(result.PassedItems, "公司名称已填写")
	}

	if vendor.UnifiedSocialCreditCode == "" {
		result.FailedItems = append(result.FailedItems, "统一社会信用代码不能为空")
	} else {
		if s.isValidUSCC(vendor.UnifiedSocialCreditCode) {
			result.PassedItems = append(result.PassedItems, "统一社会信用代码格式正确")
		} else {
			result.WarningItems = append(result.WarningItems, "统一社会信用代码格式可能有误，建议人工复核")
		}
	}

	if vendor.LegalPerson == "" {
		result.WarningItems = append(result.WarningItems, "法人信息未填写")
	} else {
		result.PassedItems = append(result.PassedItems, "法人信息已填写")
	}

	if vendor.ContactPerson == "" {
		result.FailedItems = append(result.FailedItems, "联系人不能为空")
	} else {
		result.PassedItems = append(result.PassedItems, "联系人已填写")
	}

	if vendor.ContactPhone == "" {
		result.FailedItems = append(result.FailedItems, "联系电话不能为空")
	} else {
		result.PassedItems = append(result.PassedItems, "联系电话已填写")
	}
}

func (s *ComplianceService) checkQualifications(qualifications []models.Qualification, result *models.ComplianceCheckResult) {
	hasBusinessLicense := false
	hasBankAccount := false

	for _, q := range qualifications {
		switch q.Type {
		case models.QualificationTypeBusinessLicense:
			hasBusinessLicense = true
			s.checkSingleQualification(&q, "营业执照", result)
		case models.QualificationTypeBankAccount:
			hasBankAccount = true
			s.checkSingleQualification(&q, "银行开户证明", result)
		}
	}

	if !hasBusinessLicense {
		result.FailedItems = append(result.FailedItems, "缺少营业执照")
	}
	if !hasBankAccount {
		result.WarningItems = append(result.WarningItems, "建议上传银行开户证明")
	}

	verifiedCount := 0
	for _, q := range qualifications {
		if q.Status == models.QualificationStatusVerified {
			verifiedCount++
		}
	}
	if len(qualifications) > 0 && verifiedCount == len(qualifications) {
		result.PassedItems = append(result.PassedItems, "所有资质已通过审核")
	} else if verifiedCount > 0 {
		result.WarningItems = append(result.WarningItems, "部分资质尚未审核通过")
	}
}

func (s *ComplianceService) checkSingleQualification(q *models.Qualification, name string, result *models.ComplianceCheckResult) {
	if q.FileURL == "" {
		result.FailedItems = append(result.FailedItems, name+"缺少附件")
		return
	}

	switch q.Status {
	case models.QualificationStatusVerified:
		result.PassedItems = append(result.PassedItems, name+"已通过审核")
	case models.QualificationStatusRejected:
		result.FailedItems = append(result.FailedItems, name+"审核未通过")
	case models.QualificationStatusExpired:
		result.FailedItems = append(result.FailedItems, name+"已过期")
	case models.QualificationStatusPending:
		result.WarningItems = append(result.WarningItems, name+"待审核")
	}

	if q.ExpiryDate != nil && q.ExpiryDate.Before(time.Now()) {
		result.FailedItems = append(result.FailedItems, name+"已过期")
	} else if q.ExpiryDate != nil && q.ExpiryDate.Before(time.Now().AddDate(0, 3, 0)) {
		result.WarningItems = append(result.WarningItems, name+"将在3个月内到期")
	}
}

func (s *ComplianceService) isValidUSCC(code string) bool {
	matched, _ := regexp.MatchString(`^[0-9A-HJ-NPQRTUWXY]{2}\d{6}[0-9A-HJ-NPQRTUWXY]{10}$`, code)
	return matched
}
