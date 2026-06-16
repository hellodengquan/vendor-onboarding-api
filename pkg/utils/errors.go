package utils

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/validators"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Rule    string `json:"rule"`
}

type ValidationError struct {
	Errors []FieldError `json:"errors"`
}

func (e *ValidationError) Error() string {
	var parts []string
	for _, fe := range e.Errors {
		parts = append(parts, fmt.Sprintf("[%s] %s", fe.Field, fe.Message))
	}
	return strings.Join(parts, "; ")
}

func NewValidationError(field, message, rule string) error {
	return &ValidationError{
		Errors: []FieldError{{Field: field, Message: message, Rule: rule}},
	}
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return e.Message
}

func BadRequestError(message string) error {
	return &APIError{Code: http.StatusBadRequest, Message: message}
}

func UnauthorizedError(message string) error {
	return &APIError{Code: http.StatusUnauthorized, Message: message}
}

func ForbiddenError(message string) error {
	return &APIError{Code: http.StatusForbidden, Message: message}
}

func NotFoundError(message string) error {
	return &APIError{Code: http.StatusNotFound, Message: message}
}

func ConflictError(message string) error {
	return &APIError{Code: http.StatusConflict, Message: message}
}

func WriteError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	var ve validators.ValidationErrors
	if errors.As(err, &ve) {
		fields := make([]FieldError, 0, len(ve))
		for _, e := range ve {
			fields = append(fields, FieldError{Field: e.Field, Message: e.Message, Rule: e.Code})
		}
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "参数校验失败",
			Data: gin.H{
				"errors": fields,
			},
		})
		return
	}

	var valErr *ValidationError
	if errors.As(err, &valErr) {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "参数校验失败",
			Data: gin.H{
				"errors": valErr.Errors,
			},
		})
		return
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		c.JSON(apiErr.Code, Response{
			Code:    apiErr.Code,
			Message: apiErr.Message,
		})
		return
	}

	c.JSON(http.StatusBadRequest, Response{
		Code:    400,
		Message: err.Error(),
	})
}
