package handlers

import (
	"github.com/gin-gonic/gin"

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
	req, err := h.vendorService.BindCreate(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	vendor, err := h.vendorService.Create(req)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, vendor)
}

func (h *VendorHandler) Get(c *gin.Context) {
	id, err := h.vendorService.BindGetOrSubmit(c)
	if err != nil {
		utils.WriteError(c, err)
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
	req, err := h.vendorService.BindList(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	result, err := h.vendorService.List(req)
	if err != nil {
		utils.InternalError(c, "查询失败")
		return
	}
	utils.Success(c, result)
}

func (h *VendorHandler) Update(c *gin.Context) {
	id, req, err := h.vendorService.BindUpdate(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	vendor, err := h.vendorService.Update(id, req)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, vendor)
}

func (h *VendorHandler) Submit(c *gin.Context) {
	id, err := h.vendorService.BindGetOrSubmit(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	submitterID, _ := c.Get("user_id")
	var uid uint64 = 0
	if submitterID != nil {
		uid = submitterID.(uint64)
	}
	if err := h.vendorService.Submit(id, uid); err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, gin.H{"message": "提交成功"})
}
