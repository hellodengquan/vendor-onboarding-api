package handlers

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/internal/services"
	"vendor-onboarding-api/internal/storage"
	"vendor-onboarding-api/pkg/utils"
)

type QualificationHandler struct {
	qualificationService *services.QualificationService
	fileService          *services.FileService
	fileServiceV2        *services.FileServiceV2
}

func NewQualificationHandler() *QualificationHandler {
	store, _ := storage.NewStorageFromEnv()
	return &QualificationHandler{
		qualificationService: services.NewQualificationService(),
		fileService:          services.NewFileService(),
		fileServiceV2:        services.NewFileServiceV2(store),
	}
}

func (h *QualificationHandler) Upload(c *gin.Context) {
	vendorID, err := strconv.ParseUint(c.Param("vendor_id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的供应商ID")
		return
	}

	var req models.QualificationUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	q, err := h.qualificationService.Upload(vendorID, &req)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, q)
}

func (h *QualificationHandler) UploadFile(c *gin.Context) {
	vendorID, err := strconv.ParseUint(c.Param("vendor_id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的供应商ID")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		utils.BadRequest(c, "缺少文件字段 file: "+err.Error())
		return
	}

	qTypeStr := c.PostForm("type")
	if qTypeStr == "" {
		utils.BadRequest(c, "缺少 type 字段")
		return
	}
	qType := models.QualificationType(qTypeStr)

	name := c.PostForm("name")
	if name == "" {
		name = file.Filename
	}
	number := c.PostForm("number")
	issuedBy := c.PostForm("issued_by")

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

	opts := services.DefaultProcessOptions()
	if v := c.PostForm("skip_virus_scan"); v == "1" || v == "true" {
		opts.SkipVirusScan = true
	}
	if v := c.PostForm("skip_image_crop"); v == "1" || v == "true" {
		opts.SkipImageCrop = true
	}
	if v := c.PostForm("skip_pdf_sanitize"); v == "1" || v == "true" {
		opts.SkipPDFSanitize = true
	}

	result, err := h.fileServiceV2.UploadQualificationFile(
		c.Request.Context(),
		vendorID, file, qType, name, number, issuedBy, issuedDate, expiryDate, opts,
	)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, result)
}

func (h *QualificationHandler) ListByVendor(c *gin.Context) {
	vendorID, err := strconv.ParseUint(c.Param("vendor_id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的供应商ID")
		return
	}

	list, err := h.qualificationService.GetByVendorID(vendorID)
	if err != nil {
		utils.InternalError(c, "查询失败")
		return
	}

	utils.Success(c, list)
}

func (h *QualificationHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	q, err := h.qualificationService.GetByID(id)
	if err != nil {
		utils.NotFound(c, "资质文件不存在")
		return
	}

	utils.Success(c, q)
}

func (h *QualificationHandler) Verify(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	var req models.QualificationVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	verifierID, _ := c.Get("user_id")
	var uid uint64 = 0
	if verifierID != nil {
		uid = verifierID.(uint64)
	}

	q, err := h.qualificationService.Verify(id, uid, &req)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, q)
}

func (h *QualificationHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	if err := h.qualificationService.Delete(id); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "删除成功"})
}
