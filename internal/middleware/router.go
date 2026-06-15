package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"os"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/handlers"
	"vendor-onboarding-api/internal/services"
	"vendor-onboarding-api/pkg/utils"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	fileService := services.NewFileService()
	uploadDir := fileService.GetUploadDir()
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		os.MkdirAll(uploadDir, 0755)
	}
	r.Static("/uploads", uploadDir)

	r.Use(CORS())
	r.Use(Auth())

	vendorHandler := handlers.NewVendorHandler()
	qualificationHandler := handlers.NewQualificationHandler()
	complianceHandler := handlers.NewComplianceHandler()
	approvalHandler := handlers.NewApprovalHandler()

	api := r.Group("/api/v1")
	{
		api.GET("/signature/generate", generateSignatureHelper)

		approvalNodes := api.Group("/approval-nodes")
		{
			approvalNodes.POST("", approvalHandler.CreateNodeConfig)
			approvalNodes.GET("", approvalHandler.ListNodeConfigs)
		}

		vendors := api.Group("/vendors")
		{
			vendors.POST("", vendorHandler.Create)
			vendors.GET("", vendorHandler.List)
			vendors.GET("/:id", vendorHandler.Get)
			vendors.PUT("/:id", vendorHandler.Update)
			vendors.POST("/:id/submit", vendorHandler.Submit)

			qualifications := vendors.Group("/:vendor_id/qualifications")
			{
				qualifications.POST("", qualificationHandler.Upload)
				qualifications.POST("/upload", qualificationHandler.UploadFile)
				qualifications.GET("", qualificationHandler.ListByVendor)
			}

			compliance := vendors.Group("/:vendor_id/compliance")
			{
				compliance.POST("/check", complianceHandler.Check)
			}

			approval := vendors.Group("/:vendor_id/approval")
			{
				approval.GET("/flow", approvalHandler.GetFlow)
				approval.GET("/stage/:stage/sign-status", approvalHandler.GetStageSignStatus)
				approval.POST("/approve", approvalHandler.Approve)
				approval.POST("/reject", approvalHandler.Reject)
				approval.POST("/transition", approvalHandler.Transition)
			}
		}

		qualifications := api.Group("/qualifications")
		{
			qualifications.GET("/:id", qualificationHandler.Get)
			qualifications.POST("/:id/verify", qualificationHandler.Verify)
			qualifications.DELETE("/:id", qualificationHandler.Delete)
		}
	}

	return r
}

func generateSignatureHelper(c *gin.Context) {
	method := c.Query("method")
	if method == "" {
		method = "GET"
	}
	path := c.Query("path")
	if path == "" {
		path = "/api/v1/vendors"
	}
	appKey := c.Query("app_key")
	if appKey == "" {
		appKey = "vendor_admin"
	}
	appSecret := c.Query("app_secret")
	if appSecret == "" {
		appSecret = "vendor-onboarding-secret-2024"
	}
	body := c.Query("body")

	nonceBytes := make([]byte, 8)
	rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)

	sig, ts := utils.GenerateSignature(method, path, map[string]string{}, body, appKey, appSecret, nonce)

	utils.Success(c, gin.H{
		"method":     method,
		"path":       path,
		"app_key":    appKey,
		"timestamp":  ts,
		"nonce":      nonce,
		"signature":  sig,
		"headers": gin.H{
			utils.SignatureAppKeyKey:    appKey,
			utils.SignatureTimestampKey: ts,
			utils.SignatureNonceKey:     nonce,
			utils.SignatureHeaderKey:    sig,
		},
	})
}
