package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func Fail(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: message,
	})
}

func BadRequest(c *gin.Context, message string) {
	Fail(c, 400, message)
}

func Unauthorized(c *gin.Context, message string) {
	Fail(c, 401, message)
}

func Forbidden(c *gin.Context, message string) {
	Fail(c, 403, message)
}

func NotFound(c *gin.Context, message string) {
	Fail(c, 404, message)
}

func InternalError(c *gin.Context, message string) {
	Fail(c, 500, message)
}
