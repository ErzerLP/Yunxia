package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	appsvc "yunxia/internal/application/service"
)

// WebhookSender 负责向外部 webhook 投递通知。
type WebhookSender struct {
	client *http.Client
}

// NewWebhookSender 创建 webhook 投递器。
func NewWebhookSender(timeout time.Duration) *WebhookSender {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &WebhookSender{client: &http.Client{Timeout: timeout}}
}

// Send POST 通知载荷到 webhook 地址。
func (s *WebhookSender) Send(ctx context.Context, endpoint appsvc.NotificationWebhookEndpoint, payload appsvc.NotificationWebhookPayload) error {
	if strings.TrimSpace(endpoint.URL) == "" {
		return fmt.Errorf("webhook url empty")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if endpoint.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, endpoint.Timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Yunxia-Notification/1.0")
	if strings.TrimSpace(endpoint.Secret) != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		req.Header.Set("X-Yunxia-Timestamp", timestamp)
		req.Header.Set("X-Yunxia-Signature", signWebhookPayload(endpoint.Secret, timestamp, data))
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

func signWebhookPayload(secret string, timestamp string, data []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(data)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
