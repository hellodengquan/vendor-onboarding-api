package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDStr := c.GetHeader("X-User-ID")
		userName := c.GetHeader("X-User-Name")

		if userIDStr != "" {
			if userID, err := strconv.ParseUint(userIDStr, 10, 64); err == nil {
				c.Set("user_id", userID)
			}
		}
		if userName != "" {
			c.Set("user_name", userName)
		}

		c.Next()
	}
}
