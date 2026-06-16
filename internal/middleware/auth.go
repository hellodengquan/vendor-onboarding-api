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
	ErrAPIKeyExpired    = authError("API Key 已过期，请使用新的 API Key")
	ErrSignatureMismatch = authError("签名不匹配")
)

const (
	APIKeyHeaderStatus    = "X-API-Key-Status"
	APIKeyHeaderExpiredAt = "X-API-Key-Expired-At"
	APIKeyStatusValid     = "valid"
	APIKeyStatusExpired   = "expired"
	APIKeyStatusRotating  = "rotating"
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

		result, err := validateSignatureMulti(c, cfg.APIKeySvc, sig, appKey, tsStr, nonce)
		if err != nil {
			if result != nil {
				c.Header(APIKeyHeaderStatus, result.Status)
				if result.ExpiredAt != "" {
					c.Header(APIKeyHeaderExpiredAt, result.ExpiredAt)
				}
			}
			if errors.Is(err, ErrAPIKeyExpired) {
				utils.Unauthorized(c, err.Error())
			} else {
				utils.Unauthorized(c, "认证失败: "+err.Error())
			}
			c.Abort()
			return
		}
		if result != nil {
			c.Header(APIKeyHeaderStatus, result.Status)
		}
		c.Next()
	}
}

type authResult struct {
	Status    string
	ExpiredAt string
}

func validateSignatureMulti(c *gin.Context, svc *services.APIKeyService, sig string, appKey string, tsStr string, nonce string) (*authResult, error) {
	switch {
	case appKey == "":
		return nil, ErrMissingAppKey
	case tsStr == "":
		return nil, ErrMissingTimestamp
	case nonce == "":
		return nil, ErrMissingNonce
	case sig == "":
		return nil, ErrMissingSignature
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return nil, ErrInvalidTimestamp
	}

	secretMap, err := svc.GetActiveSecrets(appKey)
	if err != nil || len(secretMap) == 0 {
		expiredInfo, hasExpired := svc.GetLatestExpiredInfo(appKey)
		if hasExpired {
			return &authResult{
				Status:    APIKeyStatusExpired,
				ExpiredAt: expiredInfo,
			}, ErrAPIKeyExpired
		}
		return nil, ErrInvalidAppKey
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

	status := APIKeyStatusValid
	if hasRotating, _ := svc.HasRotatingKey(appKey); hasRotating {
		status = APIKeyStatusRotating
	}

	var lastErr error
	for _, secret := range secretMap {
		msg := utils.BuildSignatureString(method, path, queryMap, bodyStr, ts, nonce, appKey)
		expected := utils.HMACSHA256(msg, secret)
		if hmac.Equal([]byte(expected), []byte(sig)) {
			return &authResult{Status: status}, nil
		}
		lastErr = ErrSignatureMismatch
	}
	if lastErr != nil {
		return &authResult{Status: status}, lastErr
	}
	return &authResult{Status: status}, errors.New("签名校验失败")
}

var _ = strings.TrimSpace
