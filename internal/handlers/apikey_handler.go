package handlers

import (
	"github.com/gin-gonic/gin"

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
	req, err := h.svc.BindCreate(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	key, secret, err := h.svc.Create(req.AppKey, req.Description)
	if err != nil {
		utils.WriteError(c, err)
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
	req, err := h.svc.BindRotate(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	hours := req.ExpireOldHours
	if hours <= 0 {
		hours = 72
	}
	key, newSecret, err := h.svc.Rotate(req.AppKey, hours, req.Remark)
	if err != nil {
		utils.WriteError(c, err)
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
	appKey, version, err := h.svc.BindRevoke(c)
	if err != nil {
		utils.WriteError(c, err)
		return
	}
	if err := h.svc.Revoke(appKey, version); err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, gin.H{"message": "revoked"})
}

func (h *APIKeyHandler) Cleanup(c *gin.Context) {
	rotated, purged, err := h.svc.CleanupExpiredFull(90)
	if err != nil {
		utils.InternalError(c, err.Error())
		return
	}
	utils.Success(c, gin.H{
		"rotated_expired": rotated,
		"revoked_purged":  purged,
	})
}
