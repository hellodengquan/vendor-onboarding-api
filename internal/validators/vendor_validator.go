package validators

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/models"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Field, e.Message)
}

type ValidationErrors []*ValidationError

func (errs ValidationErrors) Error() string {
	var parts []string
	for _, e := range errs {
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, "; ")
}

func (errs ValidationErrors) HasErrors() bool { return len(errs) > 0 }

func (errs *ValidationErrors) Add(field, msg, code string) {
	*errs = append(*errs, &ValidationError{Field: field, Message: msg, Code: code})
}

type VendorValidator struct{}

func NewVendorValidator() *VendorValidator { return &VendorValidator{} }

func (v *VendorValidator) ValidateCreate(req *models.VendorCreateRequest) error {
	errs := ValidationErrors{}

	if strings.TrimSpace(req.CompanyName) == "" {
		errs.Add("company_name", "公司名称不能为空", "REQUIRED")
	} else if len(req.CompanyName) > 256 {
		errs.Add("company_name", "公司名称不能超过256字符", "LENGTH")
	}

	if req.UnifiedSocialCreditCode == "" {
		errs.Add("unified_social_credit_code", "统一社会信用代码不能为空", "REQUIRED")
	} else if !isValidUSCC(req.UnifiedSocialCreditCode) {
		errs.Add("unified_social_credit_code", "统一社会信用代码格式错误（应为18位）", "FORMAT")
	} else {
		db := database.GetDB()
		var cnt int64
		db.Model(&models.Vendor{}).Where("unified_social_credit_code = ?", req.UnifiedSocialCreditCode).Count(&cnt)
		if cnt > 0 {
			errs.Add("unified_social_credit_code", "该信用代码已被注册", "DUPLICATE")
		}
	}

	if strings.TrimSpace(req.ContactPerson) == "" {
		errs.Add("contact_person", "联系人不能为空", "REQUIRED")
	}
	if req.ContactPhone == "" {
		errs.Add("contact_phone", "联系电话不能为空", "REQUIRED")
	} else if !isValidPhone(req.ContactPhone) {
		errs.Add("contact_phone", "联系电话格式错误", "FORMAT")
	}
	if req.ContactEmail != "" && !isValidEmail(req.ContactEmail) {
		errs.Add("contact_email", "邮箱格式错误", "FORMAT")
	}
	if errs.HasErrors() {
		return errs
	}
	return nil
}

func (v *VendorValidator) ValidateUpdate(id uint64, req *models.VendorUpdateRequest) error {
	errs := ValidationErrors{}
	db := database.GetDB()
	var vendor models.Vendor
	if err := db.First(&vendor, id).Error; err != nil {
		return errors.New("供应商不存在")
	}
	if vendor.Status != models.VendorStatusDraft && vendor.Status != models.VendorStatusRejected {
		return errors.New("当前状态不允许编辑")
	}
	if req.ContactPhone != "" && !isValidPhone(req.ContactPhone) {
		errs.Add("contact_phone", "联系电话格式错误", "FORMAT")
	}
	if req.ContactEmail != "" && !isValidEmail(req.ContactEmail) {
		errs.Add("contact_email", "邮箱格式错误", "FORMAT")
	}
	if req.UnifiedSocialCreditCode != "" {
		if !isValidUSCC(req.UnifiedSocialCreditCode) {
			errs.Add("unified_social_credit_code", "信用代码格式错误", "FORMAT")
		} else {
			var cnt int64
			db.Model(&models.Vendor{}).Where("unified_social_credit_code = ? AND id != ?", req.UnifiedSocialCreditCode, id).Count(&cnt)
			if cnt > 0 {
				errs.Add("unified_social_credit_code", "该信用代码已被其他供应商使用", "DUPLICATE")
			}
		}
	}
	if errs.HasErrors() {
		return errs
	}
	return nil
}

func (v *VendorValidator) ValidateSubmit(id uint64) error {
	db := database.GetDB()
	var vendor models.Vendor
	if err := db.First(&vendor, id).Error; err != nil {
		return errors.New("供应商不存在")
	}
	if vendor.Status != models.VendorStatusDraft && vendor.Status != models.VendorStatusRejected {
		return errors.New("当前状态不允许提交")
	}
	errs := ValidationErrors{}
	if vendor.CompanyName == "" {
		errs.Add("company_name", "提交前公司名称必填", "SUBMIT_REQUIRED")
	}
	if vendor.UnifiedSocialCreditCode == "" {
		errs.Add("unified_social_credit_code", "提交前信用代码必填", "SUBMIT_REQUIRED")
	}
	if vendor.ContactPerson == "" {
		errs.Add("contact_person", "提交前联系人必填", "SUBMIT_REQUIRED")
	}
	if vendor.ContactPhone == "" {
		errs.Add("contact_phone", "提交前电话必填", "SUBMIT_REQUIRED")
	}
	var qCnt int64
	db.Model(&models.Qualification{}).Where("vendor_id = ? AND type = ?", id, models.QualificationTypeBusinessLicense).Count(&qCnt)
	if qCnt == 0 {
		errs.Add("qualifications", "必须上传营业执照", "SUBMIT_REQUIRED")
	}
	if errs.HasErrors() {
		return errs
	}
	return nil
}

var usccRegex = regexp.MustCompile(`^[0-9A-HJ-NPQRTUWXY]{2}\d{6}[0-9A-HJ-NPQRTUWXY]{10}$`)
var phoneRegex = regexp.MustCompile(`^1[3-9]\d{9}$|^0\d{2,3}-?\d{7,8}$`)
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func isValidUSCC(code string) bool {
	return len(code) == 18 && usccRegex.MatchString(code)
}

func isValidPhone(p string) bool {
	return phoneRegex.MatchString(strings.ReplaceAll(p, " ", ""))
}

func isValidEmail(e string) bool { return emailRegex.MatchString(e) }

type QualificationValidator struct{}

func NewQualificationValidator() *QualificationValidator { return &QualificationValidator{} }

func (v *QualificationValidator) ValidateUpload(qType models.QualificationType, size int64, ext string) error {
	errs := ValidationErrors{}
	if size <= 0 {
		errs.Add("file", "文件不能为空", "EMPTY")
	}
	if size > 10*1024*1024 {
		errs.Add("file", "文件不能超过10MB", "SIZE")
	}
	allowedExt := map[string]bool{
		".pdf": true, ".jpg": true, ".jpeg": true, ".png": true,
		".gif": true, ".bmp": true, ".doc": true, ".docx": true,
		".xls": true, ".xlsx": true, ".tiff": true,
	}
	if !allowedExt[strings.ToLower(ext)] {
		errs.Add("file", "不支持的文件类型: "+ext, "TYPE")
	}
	validTypes := map[models.QualificationType]bool{
		models.QualificationTypeBusinessLicense:  true,
		models.QualificationTypeTaxCertificate:   true,
		models.QualificationTypeBankAccount:      true,
		models.QualificationTypeIDCard:           true,
		models.QualificationTypeOrganizationCode: true,
		models.QualificationTypeOther:            true,
	}
	if !validTypes[qType] {
		errs.Add("type", "无效的资质类型", "TYPE")
	}
	if errs.HasErrors() {
		return errs
	}
	return nil
}

type ApprovalValidator struct{}

func NewApprovalValidator() *ApprovalValidator { return &ApprovalValidator{} }

func (v *ApprovalValidator) ValidateApprove(vendorID uint64, approverID uint64) error {
	db := database.GetDB()
	errs := ValidationErrors{}
	var flow models.ApprovalFlow
	if err := db.Where("vendor_id = ?", vendorID).First(&flow).Error; err != nil {
		return errors.New("流程不存在")
	}
	if flow.Status == models.ApprovalStatusApproved {
		errs.Add("status", "流程已完成", "ALREADY_DONE")
	}
	if flow.Status == models.ApprovalStatusRejected {
		errs.Add("status", "流程已被驳回", "ALREADY_DONE")
	}
	if flow.Status == models.ApprovalStatusWithdrawn {
		errs.Add("status", "流程已撤回", "ALREADY_DONE")
	}
	var existing models.ApprovalRecord
	if err := db.Where("vendor_id = ? AND stage = ? AND approver_id = ? AND status IN ?",
		vendorID, flow.CurrentStage, approverID,
		[]models.ApprovalStatus{models.ApprovalStatusApproved, models.ApprovalStatusRejected}).
		First(&existing).Error; err == nil {
		errs.Add("approver", "该审批人已签署", "DUPLICATE")
	}
	if errs.HasErrors() {
		return errs
	}
	return nil
}
