package handlers

import (
	"strconv"

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
	vendorID, req, err := h.qualificationService.BindUpload(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	q, err := h.qualificationService.Upload(vendorID, req)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, q)
}

func (h *QualificationHandler) UploadFile(c *gin.Context) {
	vendorID, req, opts, err := h.qualificationService.BindUploadFile(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	result, err := h.fileServiceV2.UploadQualificationFile(
		c.Request.Context(),
		vendorID, req.File, req.Type, req.Name, req.Number, req.IssuedBy,
		req.IssuedDate, req.ExpiryDate, opts,
	)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, result)
}

func (h *QualificationHandler) ListByVendor(c *gin.Context) {
	vendorID, _, err := h.qualificationService.BindListOrDelete(c)
	if err != nil {
		utils.WriteError(c, err)
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
	id, err := strconv.ParseUint(c.Param("qid"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的资质ID")
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
	id, err := strconv.ParseUint(c.Param("qid"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的资质ID")
		return
	}

	var req models.QualificationVerifyRequest
	if _, err := services.BindAndValidate(c, &req); err != nil {
		utils.WriteError(c, err)
		return
	}

	verifierID, _ := c.Get("user_id")
	var uid uint64 = 0
	if verifierID != nil {
		uid = verifierID.(uint64)
	}

	q, err := h.qualificationService.Verify(id, uid, &req)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, q)
}

func (h *QualificationHandler) Delete(c *gin.Context) {
	_, qID, err := h.qualificationService.BindListOrDelete(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	if qID == 0 {
		utils.BadRequest(c, "缺少资质ID")
		return
	}
	if err := h.qualificationService.Delete(qID); err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, gin.H{"message": "删除成功"})
}
