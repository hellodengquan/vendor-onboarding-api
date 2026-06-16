package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/internal/services"
	"vendor-onboarding-api/pkg/utils"
)

type APIKeyHandler struct {
	svc *services.APIKeyService
}

func NewAPIKeyHandler() *APIKeyHandler {
	return &APIKeyHandler{svc: services.NewAPIKeyService()}
}

func (h *APIKeyHandler) Create(c *gin.Context) {
	type req struct {
		AppKey string `json:"app_key"`
		Remark string `json:"remark"`
	}
	var r req
	if err := c.ShouldBindJSON(&r); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	key, secret, err := h.svc.Create(r.AppKey, r.Remark)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, gin.H{
		"id":         key.ID,
		"app_key":    key.AppKey,
		"app_secret": secret,
		"version":    key.Version,
		"remark":     key.Remark,
		"created_at": key.CreatedAt,
	})
}

func (h *APIKeyHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, list)
}

func (h *APIKeyHandler) Rotate(c *gin.Context) {
	var req models.APIKeyRotateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	hours := req.OldVersion
	if hours <= 0 {
		hours = 24
	}
	key, newSecret, err := h.svc.Rotate(req.AppKey, hours, req.Remark)
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.Success(c, gin.H{
		"app_key":        key.AppKey,
		"new_app_secret": newSecret,
		"new_version":    key.Version,
		"old_expire_hours": hours,
		"remark":         key.Remark,
	})
}

func (h *APIKeyHandler) Revoke(c *gin.Context) {
	appKey := c.Param("app_key")
	versionStr := c.Query("version")
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		utils.BadRequest(c, "invalid version")
		return
	}
	if err := h.svc.Revoke(appKey, version); err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"message": "revoked"})
}

func (h *APIKeyHandler) Cleanup(c *gin.Context) {
	n, err := h.svc.CleanupExpired()
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"cleaned_count": n})
}
