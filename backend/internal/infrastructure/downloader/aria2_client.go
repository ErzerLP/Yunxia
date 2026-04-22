package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"time"

	appsvc "yunxia/internal/application/service"
)

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type aria2Status struct {
	Status          string `json:"status"`
	CompletedLength string `json:"completedLength"`
	TotalLength     string `json:"totalLength"`
	DownloadSpeed   string `json:"downloadSpeed"`
	ErrorMessage    string `json:"errorMessage"`
	Files           []struct {
		Path string `json:"path"`
	} `json:"files"`
}

// Aria2Client 实现 Aria2 JSON-RPC 下载器。
type Aria2Client struct {
	rpcURL string
	secret string
	client *http.Client
}

// NewAria2Client 创建客户端。
func NewAria2Client(rpcURL, secret string) *Aria2Client {
	return &Aria2Client{
		rpcURL: rpcURL,
		secret: secret,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// AddURI 创建离线下载任务。
func (c *Aria2Client) AddURI(ctx context.Context, uri string, dir string) (string, error) {
	options := map[string]any{}
	if dir != "" {
		options["dir"] = dir
	}

	resp, err := c.call(ctx, rpcRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "aria2.addUri",
		Params:  c.params([]string{uri}, options),
	})
	if err != nil {
		return "", err
	}

	var gid string
	if err := json.Unmarshal(resp.Result, &gid); err != nil {
		return "", err
	}
	return gid, nil
}

// TellStatus 查询任务状态。
func (c *Aria2Client) TellStatus(ctx context.Context, externalID string) (*appsvc.DownloadStatus, error) {
	resp, err := c.call(ctx, rpcRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "aria2.tellStatus",
		Params:  c.params(externalID),
	})
	if err != nil {
		return nil, err
	}

	var raw aria2Status
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		return nil, err
	}

	completedBytes, err := parseAria2Int64(raw.CompletedLength)
	if err != nil {
		return nil, err
	}
	totalBytes, err := parseAria2OptionalInt64(raw.TotalLength)
	if err != nil {
		return nil, err
	}
	speedBytes, err := parseAria2Int64(raw.DownloadSpeed)
	if err != nil {
		return nil, err
	}

	var etaSeconds *int64
	if totalBytes != nil && speedBytes > 0 && *totalBytes >= completedBytes {
		remaining := *totalBytes - completedBytes
		value := remaining / speedBytes
		etaSeconds = &value
	}

	var displayName string
	if len(raw.Files) > 0 && raw.Files[0].Path != "" {
		displayName = path.Base(raw.Files[0].Path)
	}

	var errorMessage *string
	if raw.ErrorMessage != "" {
		errorMessage = &raw.ErrorMessage
	}

	return &appsvc.DownloadStatus{
		Status:         mapAria2Status(raw.Status),
		CompletedBytes: completedBytes,
		TotalBytes:     totalBytes,
		DownloadSpeed:  speedBytes,
		ETASeconds:     etaSeconds,
		DisplayName:    displayName,
		ErrorMessage:   errorMessage,
	}, nil
}

// Remove 移除任务。
func (c *Aria2Client) Remove(ctx context.Context, externalID string) error {
	_, err := c.call(ctx, rpcRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "aria2.remove",
		Params:  c.params(externalID),
	})
	return err
}

// Pause 暂停任务。
func (c *Aria2Client) Pause(ctx context.Context, externalID string) error {
	_, err := c.call(ctx, rpcRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "aria2.pause",
		Params:  c.params(externalID),
	})
	return err
}

// Resume 恢复任务。
func (c *Aria2Client) Resume(ctx context.Context, externalID string) error {
	_, err := c.call(ctx, rpcRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "aria2.unpause",
		Params:  c.params(externalID),
	})
	return err
}

func (c *Aria2Client) call(ctx context.Context, payload rpcRequest) (*rpcResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aria2 rpc status %d", response.StatusCode)
	}

	var rpcResp rpcResponse
	if err := json.NewDecoder(response.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("aria2 rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return &rpcResp, nil
}

func (c *Aria2Client) params(values ...any) []any {
	params := make([]any, 0, len(values)+1)
	if c.secret != "" {
		params = append(params, "token:"+c.secret)
	}
	params = append(params, values...)
	return params
}

func parseAria2Int64(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}

func parseAria2OptionalInt64(raw string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func mapAria2Status(raw string) string {
	switch raw {
	case "active":
		return "running"
	case "waiting":
		return "pending"
	case "paused":
		return "paused"
	case "complete":
		return "completed"
	case "error":
		return "failed"
	case "removed":
		return "canceled"
	default:
		return raw
	}
}
