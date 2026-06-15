package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	SignatureHeaderKey     = "X-Signature"
	SignatureTimestampKey  = "X-Timestamp"
	SignatureNonceKey      = "X-Nonce"
	SignatureAppKeyKey     = "X-App-Key"
	SignatureExpireSeconds = 300
)

func HMACSHA256(message string, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

func BuildSignatureString(method string, path string, query map[string]string, body string, timestamp int64, nonce string, appKey string) string {
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	queryParts := make([]string, 0, len(keys))
	for _, k := range keys {
		queryParts = append(queryParts, fmt.Sprintf("%s=%s", k, query[k]))
	}
	queryStr := strings.Join(queryParts, "&")

	parts := []string{
		strings.ToUpper(method),
		path,
		queryStr,
		body,
		fmt.Sprintf("%d", timestamp),
		nonce,
		appKey,
	}
	return strings.Join(parts, "|")
}

func GenerateSignature(method string, path string, query map[string]string, body string, appKey string, appSecret string, nonce string) (string, int64) {
	ts := time.Now().Unix()
	msg := BuildSignatureString(method, path, query, body, ts, nonce, appKey)
	sig := HMACSHA256(msg, appSecret)
	return sig, ts
}

func ValidateSignature(sig string, method string, path string, query map[string]string, body string, timestamp int64, nonce string, appKey string, appSecret string) error {
	if appKey == "" || appSecret == "" || sig == "" {
		return fmt.Errorf("签名参数不完整")
	}

	if time.Now().Unix()-timestamp > SignatureExpireSeconds {
		return fmt.Errorf("签名已过期")
	}
	if timestamp > time.Now().Unix()+60 {
		return fmt.Errorf("时间戳非法")
	}
	if nonce == "" {
		return fmt.Errorf("nonce 不能为空")
	}

	msg := BuildSignatureString(method, path, query, body, timestamp, nonce, appKey)
	expected := HMACSHA256(msg, appSecret)
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return fmt.Errorf("签名校验失败")
	}
	return nil
}
