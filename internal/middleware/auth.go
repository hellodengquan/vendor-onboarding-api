package middleware

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"

	"vendor-onboarding-api/pkg/utils"
)

type AppCredential struct {
	AppKey    string
	AppSecret string
}

var (
	defaultCredentials = map[string]AppCredential{}
	nonceStore         = sync.Map{}
	credOnce           sync.Once
)

func loadCredentialsFromEnv() map[string]AppCredential {
	cred := map[string]AppCredential{}
	appKey := os.Getenv("APP_KEY")
	appSecret := os.Getenv("APP_SECRET")
	if appKey != "" && appSecret != "" {
		cred[appKey] = AppCredential{AppKey: appKey, AppSecret: appSecret}
	}
	if len(cred) == 0 {
		cred["vendor_admin"] = AppCredential{
			AppKey:    "vendor_admin",
			AppSecret: "vendor-onboarding-secret-2024",
		}
	}
	return cred
}

func getDefaultCredentials() map[string]AppCredential {
	credOnce.Do(func() {
		defaultCredentials = loadCredentialsFromEnv()
	})
	return defaultCredentials
}

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

		sig := c.GetHeader(utils.SignatureHeaderKey)
		appKey := c.GetHeader(utils.SignatureAppKeyKey)
		tsStr := c.GetHeader(utils.SignatureTimestampKey)
		nonce := c.GetHeader(utils.SignatureNonceKey)

		if sig == "" && appKey == "" && tsStr == "" {
			c.Next()
			return
		}

		if err := validateRequestSignature(c, sig, appKey, tsStr, nonce); err != nil {
			utils.Unauthorized(c, "认证失败: "+err.Error())
			c.Abort()
			return
		}

		c.Next()
	}
}

func validateRequestSignature(c *gin.Context, sig string, appKey string, tsStr string, nonce string) error {
	if appKey == "" {
		return ErrMissingAppKey
	}
	if tsStr == "" {
		return ErrMissingTimestamp
	}
	if nonce == "" {
		return ErrMissingNonce
	}
	if sig == "" {
		return ErrMissingSignature
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return ErrInvalidTimestamp
	}

	creds := getDefaultCredentials()
	cred, ok := creds[appKey]
	if !ok {
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

	return utils.ValidateSignature(
		sig, method, path, queryMap, bodyStr, ts, nonce, appKey, cred.AppSecret,
	)
}

type authError string

func (e authError) Error() string { return string(e) }

const (
	ErrMissingAppKey     = authError("缺少 X-App-Key")
	ErrMissingTimestamp  = authError("缺少 X-Timestamp")
	ErrMissingNonce      = authError("缺少 X-Nonce")
	ErrMissingSignature  = authError("缺少 X-Signature")
	ErrInvalidTimestamp  = authError("时间戳格式无效")
	ErrInvalidAppKey     = authError("无效的 AppKey")
)
