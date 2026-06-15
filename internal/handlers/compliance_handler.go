package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/services"
	"vendor-onboarding-api/pkg/utils"
)

type ComplianceHandler struct {
	complianceService *services.ComplianceService
}

func NewComplianceHandler() *ComplianceHandler {
	return &ComplianceHandler{
		complianceService: services.NewComplianceService(),
	}
}

func (h *ComplianceHandler) Check(c *gin.Context) {
	vendorID, err := strconv.ParseUint(c.Param("vendor_id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的供应商ID")
		return
	}

	result, err := h.complianceService.Check(vendorID)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, result)
}
