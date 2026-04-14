package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
)

type iamLoginRequest struct {
	Method      string          `json:"method"`
	Credentials json.RawMessage `json:"credentials"`
	DeviceID    string          `json:"device_id"`
}

type iamLoginCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TenantID string `json:"tenant_id,omitempty"`
}

type iamLoginResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func fetchTokenFromIAM(ctx context.Context, cfg IAMConfig, logger log.Logger) (string, error) {
	return fetchTokenFromIAMWithPassword(ctx, cfg.LoginURL, cfg.Username, cfg.Password, cfg.TenantID, "seeddata", logger)
}

func fetchTokenFromIAMWithPassword(
	ctx context.Context,
	loginURL, username, password, tenantID, deviceID string,
	logger log.Logger,
) (string, error) {
	if strings.TrimSpace(loginURL) == "" {
		return "", fmt.Errorf("iam login url is empty")
	}
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("iam username/password is empty")
	}
	if strings.TrimSpace(deviceID) == "" {
		deviceID = "seeddata"
	}

	credBytes, err := json.Marshal(iamLoginCredentials{
		Username: username,
		Password: password,
		TenantID: strings.TrimSpace(tenantID),
	})
	if err != nil {
		return "", fmt.Errorf("marshal iam credentials: %w", err)
	}

	reqBody, err := json.Marshal(iamLoginRequest{
		Method:      "password",
		Credentials: credBytes,
		DeviceID:    deviceID,
	})
	if err != nil {
		return "", fmt.Errorf("marshal iam login request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create iam request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request iam token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read iam response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyStr := string(body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		return "", fmt.Errorf("iam login failed: status=%d body=%s", resp.StatusCode, bodyStr)
	}

	var respWrapper iamLoginResponse
	if err := json.Unmarshal(body, &respWrapper); err != nil {
		return "", fmt.Errorf("unmarshal iam response: %w", err)
	}
	if respWrapper.Code != 0 {
		return "", fmt.Errorf("iam login error: code=%d message=%s", respWrapper.Code, respWrapper.Message)
	}

	token := extractTokenFromIAMData(respWrapper.Data)
	if token == "" {
		logger.Warnw("IAM login response missing token field")
		return "", fmt.Errorf("iam login response missing token")
	}

	identity := parseSeedTokenIdentity(token)
	logger.Infow("IAM token acquired",
		"iam_username", strings.TrimSpace(username),
		"subject", identity.Subject,
		"user_id", identity.UserID,
		"account_id", identity.AccountID,
		"tenant_id", identity.TenantID,
		"expires_at", identity.ExpiresAt,
	)

	return token, nil
}

type seedTokenIdentity struct {
	Subject   string
	UserID    string
	AccountID string
	TenantID  string
	ExpiresAt time.Time
}

func parseSeedTokenIdentity(token string) seedTokenIdentity {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return seedTokenIdentity{}
	}

	payload, err := decodeSeedTokenSegment(parts[1])
	if err != nil {
		return seedTokenIdentity{}
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return seedTokenIdentity{}
	}

	return seedTokenIdentity{
		Subject:   readStringField(claims, "sub"),
		UserID:    readStringField(claims, "user_id"),
		AccountID: readStringField(claims, "account_id"),
		TenantID:  readStringField(claims, "tenant_id"),
		ExpiresAt: readUnixTimeField(claims, "exp"),
	}
}

func decodeSeedTokenSegment(segment string) ([]byte, error) {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return nil, fmt.Errorf("empty token segment")
	}
	if payload, err := base64.RawURLEncoding.DecodeString(segment); err == nil {
		return payload, nil
	}
	return base64.URLEncoding.DecodeString(segment)
}

func extractTokenFromIAMData(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}

	if token := readStringField(data, "token"); token != "" {
		return token
	}
	if token := readStringField(data, "access_token"); token != "" {
		return token
	}
	if token := readStringField(data, "accessToken"); token != "" {
		return token
	}

	if tokenPair, ok := data["token_pair"].(map[string]interface{}); ok {
		if token := readStringField(tokenPair, "access_token"); token != "" {
			return token
		}
		if token := readStringField(tokenPair, "accessToken"); token != "" {
			return token
		}
	}

	if tokenPair, ok := data["tokenPair"].(map[string]interface{}); ok {
		if token := readStringField(tokenPair, "access_token"); token != "" {
			return token
		}
		if token := readStringField(tokenPair, "accessToken"); token != "" {
			return token
		}
	}

	return ""
}

func readStringField(data map[string]interface{}, key string) string {
	if value, ok := data[key]; ok {
		if str, ok := value.(string); ok {
			return strings.TrimSpace(str)
		}
	}
	return ""
}

func readUnixTimeField(data map[string]interface{}, key string) time.Time {
	value, ok := data[key]
	if !ok || value == nil {
		return time.Time{}
	}

	switch v := value.(type) {
	case float64:
		return time.Unix(int64(v), 0).UTC()
	case int64:
		return time.Unix(v, 0).UTC()
	case int:
		return time.Unix(int64(v), 0).UTC()
	case json.Number:
		seconds, err := v.Int64()
		if err != nil {
			return time.Time{}
		}
		return time.Unix(seconds, 0).UTC()
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return time.Time{}
		}
		seconds, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return time.Time{}
		}
		return time.Unix(seconds, 0).UTC()
	default:
		return time.Time{}
	}
}
