package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/internal/services"
	"vendor-onboarding-api/pkg/utils"
)

type QualificationHandler struct {
	qualificationService *services.QualificationService
}

func NewQualificationHandler() *QualificationHandler {
	return &QualificationHandler{
		qualificationService: services.NewQualificationService(),
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
