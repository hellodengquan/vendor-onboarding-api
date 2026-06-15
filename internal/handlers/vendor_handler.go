package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/internal/services"
	"vendor-onboarding-api/pkg/utils"
)

type VendorHandler struct {
	vendorService *services.VendorService
}

func NewVendorHandler() *VendorHandler {
	return &VendorHandler{
		vendorService: services.NewVendorService(),
	}
}

func (h *VendorHandler) Create(c *gin.Context) {
	var req models.VendorCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	vendor, err := h.vendorService.Create(&req)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, vendor)
}

func (h *VendorHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	vendor, err := h.vendorService.GetByID(id)
	if err != nil {
		utils.NotFound(c, "供应商不存在")
		return
	}

	utils.Success(c, vendor)
}

func (h *VendorHandler) List(c *gin.Context) {
	var req models.VendorListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	result, err := h.vendorService.List(&req)
	if err != nil {
		utils.InternalError(c, "查询失败")
		return
	}

	utils.Success(c, result)
}

func (h *VendorHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	var req models.VendorUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	vendor, err := h.vendorService.Update(id, &req)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, vendor)
}

func (h *VendorHandler) Submit(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的ID")
		return
	}

	submitterID, _ := c.Get("user_id")
	var uid uint64 = 0
	if submitterID != nil {
		uid = submitterID.(uint64)
	}

	if err := h.vendorService.Submit(id, uid); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "提交成功"})
}
