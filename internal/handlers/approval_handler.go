package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/internal/services"
	"vendor-onboarding-api/pkg/utils"
)

type ApprovalHandler struct {
	approvalService *services.ApprovalService
}

func NewApprovalHandler() *ApprovalHandler {
	return &ApprovalHandler{
		approvalService: services.NewApprovalService(),
	}
}

func (h *ApprovalHandler) GetFlow(c *gin.Context) {
	vendorID, err := strconv.ParseUint(c.Param("vendor_id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的供应商ID")
		return
	}

	result, err := h.approvalService.GetFlow(vendorID)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, result)
}

func (h *ApprovalHandler) Approve(c *gin.Context) {
	var req models.ApprovalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	approverID, _ := c.Get("user_id")
	approverName, _ := c.Get("user_name")
	var uid uint64 = 0
	var uname string = ""
	if approverID != nil {
		uid = approverID.(uint64)
	}
	if approverName != nil {
		uname = approverName.(string)
	}

	if err := h.approvalService.Approve(&req, uid, uname); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "审批通过"})
}

func (h *ApprovalHandler) Reject(c *gin.Context) {
	var req models.ApprovalRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	approverID, _ := c.Get("user_id")
	approverName, _ := c.Get("user_name")
	var uid uint64 = 0
	var uname string = ""
	if approverID != nil {
		uid = approverID.(uint64)
	}
	if approverName != nil {
		uname = approverName.(string)
	}

	if err := h.approvalService.Reject(&req, uid, uname); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "已驳回"})
}

func (h *ApprovalHandler) Transition(c *gin.Context) {
	var req models.StageTransitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	operatorID, _ := c.Get("user_id")
	operatorName, _ := c.Get("user_name")
	var uid uint64 = 0
	var uname string = ""
	if operatorID != nil {
		uid = operatorID.(uint64)
	}
	if operatorName != nil {
		uname = operatorName.(string)
	}

	if err := h.approvalService.Transition(&req, uid, uname); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.Success(c, gin.H{"message": "阶段流转成功"})
}

func (h *ApprovalHandler) CreateNodeConfig(c *gin.Context) {
	var req models.NodeConfigCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	node, err := h.approvalService.CreateNodeConfig(&req)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, node)
}

func (h *ApprovalHandler) ListNodeConfigs(c *gin.Context) {
	list, err := h.approvalService.ListNodeConfigs()
	if err != nil {
		utils.InternalError(c, "查询失败")
		return
	}
	utils.Success(c, list)
}

func (h *ApprovalHandler) GetStageSignStatus(c *gin.Context) {
	vendorID, err := strconv.ParseUint(c.Param("vendor_id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的供应商ID")
		return
	}
	stage := models.ApprovalStage(c.Param("stage"))
	if stage == "" {
		utils.BadRequest(c, "缺少阶段参数")
		return
	}

	status, err := h.approvalService.GetStageSignStatus(vendorID, stage)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, status)
}
