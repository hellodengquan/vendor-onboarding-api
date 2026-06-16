package middleware

import (
	"bytes"
	"crypto/hmac"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/internal/services"
	"vendor-onboarding-api/pkg/utils"
)

type AuthConfig struct {
	EnforceSignature bool
	APIKeySvc        *services.APIKeyService
}

type authError string

func (e authError) Error() string { return string(e) }

const (
	ErrMissingAppKey    = authError("缺少 X-App-Key")
	ErrMissingTimestamp = authError("缺少 X-Timestamp")
	ErrMissingNonce     = authError("缺少 X-Nonce")
	ErrMissingSignature = authError("缺少 X-Signature")
	ErrInvalidTimestamp = authError("时间戳格式无效")
	ErrInvalidAppKey    = authError("无效的 AppKey")
)

var defaultAPIKeySvc = services.NewAPIKeyService()

func Auth() gin.HandlerFunc {
	return AuthWithConfig(AuthConfig{
		EnforceSignature: true,
		APIKeySvc:        defaultAPIKeySvc,
	})
}

func AuthWithConfig(cfg AuthConfig) gin.HandlerFunc {
	if cfg.APIKeySvc == nil {
		cfg.APIKeySvc = defaultAPIKeySvc
	}
	return func(c *gin.Context) {
		if userIDStr := c.GetHeader("X-User-ID"); userIDStr != "" {
			if uid, err := strconv.ParseUint(userIDStr, 10, 64); err == nil {
				c.Set("user_id", uid)
			}
		}
		if uname := c.GetHeader("X-User-Name"); uname != "" {
			c.Set("user_name", uname)
		}

		sig := c.GetHeader(utils.SignatureHeaderKey)
		appKey := c.GetHeader(utils.SignatureAppKeyKey)
		tsStr := c.GetHeader(utils.SignatureTimestampKey)
		nonce := c.GetHeader(utils.SignatureNonceKey)

		if appKey == "" && sig == "" && tsStr == "" {
			if cfg.EnforceSignature && strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
			}
			c.Next()
			return
		}

		if err := validateSignatureMulti(c, cfg.APIKeySvc, sig, appKey, tsStr, nonce); err != nil {
			utils.Unauthorized(c, "认证失败: "+err.Error())
			c.Abort()
			return
		}
		c.Next()
	}
}

func validateSignatureMulti(c *gin.Context, svc *services.APIKeyService, sig string, appKey string, tsStr string, nonce string) error {
	switch {
	case appKey == "":
		return ErrMissingAppKey
	case tsStr == "":
		return ErrMissingTimestamp
	case nonce == "":
		return ErrMissingNonce
	case sig == "":
		return ErrMissingSignature
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return ErrInvalidTimestamp
	}

	secretMap, err := svc.GetActiveSecrets(appKey)
	if err != nil || len(secretMap) == 0 {
		return ErrInvalidAppKey
	}

	queryMap := map[string]string{}
	for k, v := range c.Request.URL.Query() {
		if len(v) > 0 {
			queryMap[k] = v[0]
		}
	}

	var bodyStr string
	if c.Request.Body != nil && c.Request.Body != http.NoBody {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err == nil {
			bodyStr = string(bodyBytes)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
	}

	path := c.Request.URL.Path
	method := c.Request.Method

	var lastErr error
	for _, secret := range secretMap {
		msg := utils.BuildSignatureString(method, path, queryMap, bodyStr, ts, nonce, appKey)
		expected := utils.HMACSHA256(msg, secret)
		if hmac.Equal([]byte(expected), []byte(sig)) {
			return nil
		}
		lastErr = errors.New("签名不匹配")
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("签名校验失败")
}

var _ = strings.TrimSpace
