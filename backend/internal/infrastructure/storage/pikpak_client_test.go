package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPikPakHTTPClientOfflineDownloadEndpoints(t *testing.T) {
	var createPayload map[string]any
	var deleteQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/drive/v1/files":
			if err := json.NewDecoder(r.Body).Decode(&createPayload); err != nil {
				t.Fatalf("decode create payload error = %v", err)
			}
			_, _ = w.Write([]byte(`{"task":{"id":"task-1","name":"movie.mkv","phase":"PHASE_TYPE_PENDING","progress":0}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/drive/v1/tasks":
			if r.URL.Query().Get("type") != "offline" || r.URL.Query().Get("with") != "reference_resource" {
				t.Fatalf("unexpected status query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"tasks":[{"id":"task-1","name":"movie.mkv","phase":"PHASE_TYPE_COMPLETE","progress":1}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/drive/v1/tasks":
			deleteQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewPikPakHTTPClient(
		WithPikPakBaseURLs(server.URL, server.URL, server.URL),
		WithPikPakRetryPolicy(1, 0),
	)
	session := PikPakSession{AccessToken: "access", CaptchaToken: "captcha", DeviceID: "device", UserAgent: "ua"}

	task, err := client.CreateOfflineDownload(context.Background(), session, PikPakCreateOfflineDownloadRequest{
		ParentID: "folder-downloads",
		URL:      "https://example.com/movie.mkv",
		Name:     "movie.mkv",
	})
	if err != nil {
		t.Fatalf("CreateOfflineDownload() error = %v", err)
	}
	if task.ID != "task-1" {
		t.Fatalf("unexpected created task = %+v", task)
	}
	if createPayload["upload_type"] != "UPLOAD_TYPE_URL" || createPayload["parent_id"] != "folder-downloads" || createPayload["name"] != "movie.mkv" {
		t.Fatalf("unexpected create payload = %+v", createPayload)
	}
	urlPayload, ok := createPayload["url"].(map[string]any)
	if !ok || urlPayload["url"] != "https://example.com/movie.mkv" {
		t.Fatalf("unexpected url payload = %+v", createPayload["url"])
	}

	status, err := client.GetOfflineDownloadTask(context.Background(), session, "task-1")
	if err != nil {
		t.Fatalf("GetOfflineDownloadTask() error = %v", err)
	}
	if status.Phase != "PHASE_TYPE_COMPLETE" || status.Progress != 1 {
		t.Fatalf("unexpected status = %+v", status)
	}

	if err := client.DeleteOfflineDownloadTasks(context.Background(), session, []string{"task-1"}, true); err != nil {
		t.Fatalf("DeleteOfflineDownloadTasks() error = %v", err)
	}
	if deleteQuery == "" || !containsQueryPair(deleteQuery, "task_ids=task-1") || !containsQueryPair(deleteQuery, "delete_files=true") {
		t.Fatalf("unexpected delete query = %q", deleteQuery)
	}
}

func TestPikPakHTTPClientUsesConfiguredProxyURL(t *testing.T) {
	var proxiedURL string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedURL = r.URL.String()
		if r.Method != http.MethodPost || r.URL.Scheme != "http" || r.URL.Host != "provider.invalid" || r.URL.Path != "/v1/auth/signin" {
			t.Fatalf("unexpected proxied request %s %s", r.Method, r.URL.String())
		}
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","sub":"user-id"}`))
	}))
	defer proxy.Close()

	client := NewPikPakHTTPClient(
		WithPikPakBaseURLs("http://provider.invalid", "http://provider.invalid", "http://provider.invalid"),
		WithPikPakRetryPolicy(1, 0),
	)
	token, err := client.Login(context.Background(), PikPakConfig{
		Platform:     "web",
		Username:     "user@example.com",
		Password:     "password",
		CaptchaToken: "captcha",
		DeviceID:     "device",
		ProxyURL:     proxy.URL,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if token.AccessToken != "access" || proxiedURL == "" {
		t.Fatalf("expected login through configured proxy, token=%+v proxiedURL=%q", token, proxiedURL)
	}
}

func containsQueryPair(rawQuery string, pair string) bool {
	for _, part := range strings.Split(rawQuery, "&") {
		if part == pair {
			return true
		}
	}
	return false
}
