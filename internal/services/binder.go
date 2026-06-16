package services

import (
	"errors"
	"mime/multipart"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/pkg/utils"
)

type UploadFileRequest struct {
	File       *multipart.FileHeader `form:"file"`
	Type       models.QualificationType
	Name       string
	Number     string
	IssuedBy   string
	IssuedDate *time.Time
	ExpiryDate *time.Time
}

func (s *QualificationService) BindUploadFile(c *gin.Context) (uint64, *UploadFileRequest, ProcessOptions, error) {
	vendorID, err := parseUintParam(c, "vendor_id")
	if err != nil {
		return 0, nil, ProcessOptions{}, utils.BadRequestError("无效的供应商ID")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return 0, nil, ProcessOptions{}, utils.BadRequestError("缺少文件字段 file: " + err.Error())
	}

	qTypeStr := c.PostForm("type")
	if qTypeStr == "" {
		return 0, nil, ProcessOptions{}, utils.BadRequestError("缺少 type 字段")
	}
	qType := models.QualificationType(qTypeStr)

	name := c.PostForm("name")
	if name == "" {
		name = file.Filename
	}

	var issuedDate *time.Time
	if v := c.PostForm("issued_date"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			issuedDate = &t
		}
	}
	var expiryDate *time.Time
	if v := c.PostForm("expiry_date"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			expiryDate = &t
		}
	}

	opts := DefaultProcessOptions()
	if v := c.PostForm("skip_virus_scan"); v == "1" || v == "true" {
		opts.SkipVirusScan = true
	}
	if v := c.PostForm("skip_image_crop"); v == "1" || v == "true" {
		opts.SkipImageCrop = true
	}
	if v := c.PostForm("skip_pdf_sanitize"); v == "1" || v == "true" {
		opts.SkipPDFSanitize = true
	}

	return vendorID, &UploadFileRequest{
		File:       file,
		Type:       qType,
		Name:       name,
		Number:     c.PostForm("number"),
		IssuedBy:   c.PostForm("issued_by"),
		IssuedDate: issuedDate,
		ExpiryDate: expiryDate,
	}, opts, nil
}

type RequestBinder struct{}

func NewRequestBinder() *RequestBinder {
	return &RequestBinder{}
}

func parseUintParam(c *gin.Context, name string) (uint64, error) {
	raw := c.Param(name)
	if raw == "" {
		return 0, errors.New("missing param: " + name)
	}
	return strconv.ParseUint(raw, 10, 64)
}

type bindResult struct {
	vendorID uint64
	req      interface{}
	err      error
}

func BindAndValidate[T any](c *gin.Context, req *T) (*T, error) {
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, &utils.ValidationError{
			Errors: []utils.FieldError{{Field: "body", Message: "请求参数格式错误: " + err.Error(), Rule: "BIND"}},
		}
	}
	return req, nil
}

func (s *VendorService) BindCreate(c *gin.Context) (*models.VendorCreateRequest, error) {
	var req models.VendorCreateRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return nil, err
	}
	if err := s.v.ValidateCreate(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (s *VendorService) BindUpdate(c *gin.Context) (uint64, *models.VendorUpdateRequest, error) {
	id, err := parseUintParam(c, "vendor_id")
	if err != nil {
		return 0, nil, utils.BadRequestError("无效的供应商ID")
	}
	var req models.VendorUpdateRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return 0, nil, err
	}
	if err := s.v.ValidateUpdate(id, &req); err != nil {
		return 0, nil, err
	}
	return id, &req, nil
}

func (s *VendorService) BindList(c *gin.Context) (*models.VendorListRequest, error) {
	var req models.VendorListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		return nil, utils.BadRequestError("参数格式错误: " + err.Error())
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 200 {
		req.PageSize = 20
	}
	return &req, nil
}

func (s *VendorService) BindGetOrSubmit(c *gin.Context) (uint64, error) {
	id, err := parseUintParam(c, "vendor_id")
	if err != nil {
		return 0, utils.BadRequestError("无效的供应商ID")
	}
	if err := s.v.ValidateID(id); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *ApprovalService) BindApprove(c *gin.Context) (uint64, *models.ApprovalRequest, error) {
	vendorID, err := parseUintParam(c, "vendor_id")
	if err != nil {
		return 0, nil, utils.BadRequestError("无效的供应商ID")
	}
	var req models.ApprovalRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return 0, nil, err
	}
	req.VendorID = vendorID
	return vendorID, &req, nil
}

func (s *ApprovalService) BindReject(c *gin.Context) (uint64, *models.ApprovalRejectRequest, error) {
	vendorID, err := parseUintParam(c, "vendor_id")
	if err != nil {
		return 0, nil, utils.BadRequestError("无效的供应商ID")
	}
	var req models.ApprovalRejectRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return 0, nil, err
	}
	req.VendorID = vendorID
	if req.Remark == "" {
		return 0, nil, utils.BadRequestError("驳回原因不能为空")
	}
	return vendorID, &req, nil
}

func (s *ApprovalService) BindTransition(c *gin.Context) (uint64, *models.StageTransitionRequest, error) {
	vendorID, err := parseUintParam(c, "vendor_id")
	if err != nil {
		return 0, nil, utils.BadRequestError("无效的供应商ID")
	}
	var req models.StageTransitionRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return 0, nil, err
	}
	req.VendorID = vendorID
	if req.ToStage == "" {
		return 0, nil, utils.BadRequestError("目标阶段不能为空")
	}
	return vendorID, &req, nil
}

func (s *ApprovalService) BindAddSigner(c *gin.Context) (uint64, *models.AddSignerRequest, error) {
	vendorID, err := parseUintParam(c, "vendor_id")
	if err != nil {
		return 0, nil, utils.BadRequestError("无效的供应商ID")
	}
	var req models.AddSignerRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return 0, nil, err
	}
	req.VendorID = vendorID
	if req.ApproverID == 0 {
		return 0, nil, utils.BadRequestError("加签审批人ID不能为空")
	}
	if req.ApproverName == "" {
		return 0, nil, utils.BadRequestError("加签审批人姓名不能为空")
	}
	return vendorID, &req, nil
}

func (s *ApprovalService) BindWithdraw(c *gin.Context) (uint64, *models.WithdrawRequest, error) {
	vendorID, err := parseUintParam(c, "vendor_id")
	if err != nil {
		return 0, nil, utils.BadRequestError("无效的供应商ID")
	}
	var req models.WithdrawRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return 0, nil, err
	}
	req.VendorID = vendorID
	return vendorID, &req, nil
}

func (s *ApprovalService) BindParallelGroup(c *gin.Context) (*models.ParallelGroupCreateRequest, error) {
	var req models.ParallelGroupCreateRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return nil, err
	}
	if len(req.SubStages) < 2 {
		return nil, utils.BadRequestError("并签组至少需要两个子阶段")
	}
	return &req, nil
}

func (s *APIKeyService) BindCreate(c *gin.Context) (*models.APIKeyCreateRequest, error) {
	var req models.APIKeyCreateRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return nil, err
	}
	if req.AppKey == "" {
		return nil, utils.BadRequestError("app_key 不能为空")
	}
	return &req, nil
}

func (s *APIKeyService) BindRotate(c *gin.Context) (*models.APIKeyRotateRequest, error) {
	var req models.APIKeyRotateRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return nil, err
	}
	if req.AppKey == "" {
		return nil, utils.BadRequestError("app_key 不能为空")
	}
	if req.ExpireOldHours < 0 {
		req.ExpireOldHours = 72
	}
	return &req, nil
}

func (s *APIKeyService) BindRevoke(c *gin.Context) (string, int, error) {
	appKey := c.Param("app_key")
	if appKey == "" {
		return "", 0, utils.BadRequestError("app_key 不能为空")
	}
	version := 0
	if vs := c.Query("version"); vs != "" {
		v, err := strconv.Atoi(vs)
		if err != nil {
			return "", 0, utils.BadRequestError("无效的 version")
		}
		version = v
	}
	return appKey, version, nil
}

func (s *QualificationService) BindUpload(c *gin.Context) (uint64, *models.QualificationUploadRequest, error) {
	vendorID, err := parseUintParam(c, "vendor_id")
	if err != nil {
		return 0, nil, utils.BadRequestError("无效的供应商ID")
	}
	var req models.QualificationUploadRequest
	if _, err := BindAndValidate(c, &req); err != nil {
		return 0, nil, err
	}
	req.VendorID = vendorID
	if req.Type == "" {
		return 0, nil, utils.BadRequestError("资质类型不能为空")
	}
	if req.FileURL == "" {
		return 0, nil, utils.BadRequestError("文件URL不能为空")
	}
	return vendorID, &req, nil
}

func (s *QualificationService) BindListOrDelete(c *gin.Context) (uint64, uint64, error) {
	vendorID, err := parseUintParam(c, "vendor_id")
	if err != nil {
		return 0, 0, utils.BadRequestError("无效的供应商ID")
	}
	qID := uint64(0)
	if raw := c.Param("qid"); raw != "" {
		q, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return 0, 0, utils.BadRequestError("无效的资质ID")
		}
		qID = q
	}
	return vendorID, qID, nil
}
