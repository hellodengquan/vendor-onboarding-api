package main

import (
	"log"
	"os"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/middleware"
)

func main() {
	if err := database.Init(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	r := middleware.SetupRouter()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
