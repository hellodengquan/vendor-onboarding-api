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
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, result)
}

func (h *ApprovalHandler) Approve(c *gin.Context) {
	_, req, err := h.approvalService.BindApprove(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	uid, uname := extractUser(c)
	if err := h.approvalService.Approve(req, uid, uname); err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, gin.H{"message": "审批通过"})
}

func (h *ApprovalHandler) Reject(c *gin.Context) {
	_, req, err := h.approvalService.BindReject(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	uid, uname := extractUser(c)
	if err := h.approvalService.Reject(req, uid, uname); err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, gin.H{"message": "已驳回"})
}

func (h *ApprovalHandler) Transition(c *gin.Context) {
	_, req, err := h.approvalService.BindTransition(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	uid, uname := extractUser(c)
	if err := h.approvalService.Transition(req, uid, uname); err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, gin.H{"message": "阶段流转成功"})
}

func (h *ApprovalHandler) CreateNodeConfig(c *gin.Context) {
	var req models.NodeConfigCreateRequest
	if _, err := services.BindAndValidate(c, &req); err != nil {
		utils.WriteError(c, err)
		return
	}
	node, err := h.approvalService.CreateNodeConfig(&req)
	if err != nil {
		utils.WriteError(c, err)
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
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, status)
}

func (h *ApprovalHandler) AddSigner(c *gin.Context) {
	_, req, err := h.approvalService.BindAddSigner(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	uid, uname := extractUser(c)
	if err := h.approvalService.AddSigner(req, uid, uname); err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, gin.H{"message": "加签成功"})
}

func (h *ApprovalHandler) Withdraw(c *gin.Context) {
	_, req, err := h.approvalService.BindWithdraw(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	uid, uname := extractUser(c)
	if err := h.approvalService.Withdraw(req, uid, uname); err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, gin.H{"message": "撤回成功", "to_stage": req.ToStage})
}

func (h *ApprovalHandler) ListWithdrawals(c *gin.Context) {
	vendorID, err := strconv.ParseUint(c.Param("vendor_id"), 10, 64)
	if err != nil {
		utils.BadRequest(c, "无效的供应商ID")
		return
	}
	list, err := h.approvalService.ListWithdrawals(vendorID)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, list)
}

func (h *ApprovalHandler) CreateParallelGroup(c *gin.Context) {
	req, err := h.approvalService.BindParallelGroup(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	group, err := h.approvalService.CreateParallelGroup(req)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	utils.Success(c, group)
}

func extractUser(c *gin.Context) (uint64, string) {
	var uid uint64 = 0
	var uname string = ""
	if v, ok := c.Get("user_id"); ok && v != nil {
		uid = v.(uint64)
	}
	if v, ok := c.Get("user_name"); ok && v != nil {
		uname = v.(string)
	}
	return uid, uname
}
