package downloader

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type capturedRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

func TestAria2ClientAddURIReturnsGID(t *testing.T) {
	var captured capturedRPCRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":"gid-001"}`))
	}))
	defer server.Close()

	client := NewAria2Client(server.URL, "secret-token")
	gid, err := client.AddURI(context.Background(), "https://example.com/archive.zip", "/downloads")
	if err != nil {
		t.Fatalf("AddURI() error = %v", err)
	}
	if gid != "gid-001" {
		t.Fatalf("expected gid-001, got %q", gid)
	}
	if captured.Method != "aria2.addUri" {
		t.Fatalf("expected aria2.addUri, got %q", captured.Method)
	}
	if len(captured.Params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(captured.Params))
	}
	if captured.Params[0] != "token:secret-token" {
		t.Fatalf("expected token param, got %#v", captured.Params[0])
	}
}

func TestAria2ClientTellStatusMapsFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"jsonrpc":"2.0",
			"id":"1",
			"result":{
				"status":"active",
				"completedLength":"25",
				"totalLength":"100",
				"downloadSpeed":"5",
				"files":[{"path":"/downloads/archive.zip"}]
			}
		}`))
	}))
	defer server.Close()

	client := NewAria2Client(server.URL, "")
	status, err := client.TellStatus(context.Background(), "gid-001")
	if err != nil {
		t.Fatalf("TellStatus() error = %v", err)
	}
	if status.Status != "running" {
		t.Fatalf("expected running status, got %q", status.Status)
	}
	if status.CompletedBytes != 25 {
		t.Fatalf("expected completed bytes 25, got %d", status.CompletedBytes)
	}
	if status.TotalBytes == nil || *status.TotalBytes != 100 {
		t.Fatalf("expected total bytes 100, got %+v", status.TotalBytes)
	}
	if status.DisplayName != "archive.zip" {
		t.Fatalf("expected display name archive.zip, got %q", status.DisplayName)
	}
	if status.ETASeconds == nil || *status.ETASeconds != 15 {
		t.Fatalf("expected eta 15, got %+v", status.ETASeconds)
	}
}

func TestAria2ClientRemoveCallsRemoveMethod(t *testing.T) {
	var captured capturedRPCRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":"gid-001"}`))
	}))
	defer server.Close()

	client := NewAria2Client(server.URL, "")
	if err := client.Remove(context.Background(), "gid-001"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if captured.Method != "aria2.remove" {
		t.Fatalf("expected aria2.remove, got %q", captured.Method)
	}
}

func TestAria2ClientPauseCallsPauseMethod(t *testing.T) {
	var captured capturedRPCRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":"gid-001"}`))
	}))
	defer server.Close()

	client := NewAria2Client(server.URL, "")
	if err := client.Pause(context.Background(), "gid-001"); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	if captured.Method != "aria2.pause" {
		t.Fatalf("expected aria2.pause, got %q", captured.Method)
	}
}

func TestAria2ClientResumeCallsUnpauseMethod(t *testing.T) {
	var captured capturedRPCRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("io.ReadAll() error = %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":"gid-001"}`))
	}))
	defer server.Close()

	client := NewAria2Client(server.URL, "")
	if err := client.Resume(context.Background(), "gid-001"); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if captured.Method != "aria2.unpause" {
		t.Fatalf("expected aria2.unpause, got %q", captured.Method)
	}
}
