package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
}

type iamLoginResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func fetchTokenFromIAM(ctx context.Context, cfg IAMConfig, logger log.Logger) (string, error) {
	if strings.TrimSpace(cfg.LoginURL) == "" {
		return "", fmt.Errorf("iam login url is empty")
	}
	if strings.TrimSpace(cfg.Username) == "" || strings.TrimSpace(cfg.Password) == "" {
		return "", fmt.Errorf("iam username/password is empty")
	}

	credBytes, err := json.Marshal(iamLoginCredentials{
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		return "", fmt.Errorf("marshal iam credentials: %w", err)
	}

	reqBody, err := json.Marshal(iamLoginRequest{
		Method:      "password",
		Credentials: credBytes,
		DeviceID:    "seeddata",
	})
	if err != nil {
		return "", fmt.Errorf("marshal iam login request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.LoginURL, bytes.NewReader(reqBody))
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

	return token, nil
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
