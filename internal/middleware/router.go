package middleware

import (
	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/handlers"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	r.Use(CORS())
	r.Use(Auth())

	vendorHandler := handlers.NewVendorHandler()
	qualificationHandler := handlers.NewQualificationHandler()
	complianceHandler := handlers.NewComplianceHandler()
	approvalHandler := handlers.NewApprovalHandler()

	api := r.Group("/api/v1")
	{
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
				qualifications.GET("", qualificationHandler.ListByVendor)
			}

			compliance := vendors.Group("/:vendor_id/compliance")
			{
				compliance.POST("/check", complianceHandler.Check)
			}

			approval := vendors.Group("/:vendor_id/approval")
			{
				approval.GET("/flow", approvalHandler.GetFlow)
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
