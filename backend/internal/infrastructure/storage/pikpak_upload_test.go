package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainstorage "yunxia/internal/domain/storage"
)

func TestPikPakGCIDUploadHashCalculator(t *testing.T) {
	localPath := writeTempPikPakUploadFile(t, "abc")

	got, err := (PikPakGCIDUploadHashCalculator{}).HashFile(context.Background(), localPath)
	if err != nil {
		t.Fatalf("HashFile() error = %v", err)
	}
	const want = "0D3CED9BEC10A777AEC23CCC353A8C08A633045E"
	if got != want {
		t.Fatalf("HashFile() = %q, want %q", got, want)
	}
}

func TestPikPakHTTPOSSUploaderPutObjectSendsSignedRequest(t *testing.T) {
	localPath := writeTempPikPakUploadFile(t, "oss-body")
	var capturedPath, capturedAuth, capturedToken, capturedDate, capturedContentType, capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		capturedPath = r.URL.EscapedPath()
		capturedAuth = r.Header.Get("Authorization")
		capturedToken = r.Header.Get("X-OSS-Security-Token")
		capturedDate = r.Header.Get("Date")
		capturedContentType = r.Header.Get("Content-Type")
		capturedBody = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	uploader := NewPikPakHTTPOSSUploader(
		WithPikPakOSSHTTPClient(server.Client()),
		WithPikPakOSSNow(func() time.Time { return time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC) }),
	)
	err := uploader.PutObject(context.Background(), PikPakOSSUploadParams{
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
		Bucket:          "bucket",
		Endpoint:        server.URL,
		Key:             "dir/object.txt",
		SecurityToken:   "security-token",
	}, localPath, "text/plain")
	if err != nil {
		t.Fatalf("PutObject() error = %v", err)
	}
	if capturedPath != "/bucket/dir/object.txt" {
		t.Fatalf("unexpected request path %q", capturedPath)
	}
	if !strings.HasPrefix(capturedAuth, "OSS ak:") {
		t.Fatalf("missing OSS authorization, got %q", capturedAuth)
	}
	if capturedToken != "security-token" {
		t.Fatalf("missing security token header, got %q", capturedToken)
	}
	if capturedDate != "Mon, 04 May 2026 12:00:00 GMT" {
		t.Fatalf("unexpected Date header %q", capturedDate)
	}
	if capturedContentType != "text/plain" {
		t.Fatalf("unexpected content type %q", capturedContentType)
	}
	if capturedBody != "oss-body" {
		t.Fatalf("unexpected request body %q", capturedBody)
	}
}

func TestPikPakHTTPOSSUploaderMapsProviderErrors(t *testing.T) {
	localPath := writeTempPikPakUploadFile(t, "oss-body")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	uploader := NewPikPakHTTPOSSUploader(WithPikPakOSSHTTPClient(server.Client()))
	err := uploader.PutObject(context.Background(), PikPakOSSUploadParams{
		AccessKeyID:     "ak",
		AccessKeySecret: "sk",
		Bucket:          "bucket",
		Endpoint:        server.URL,
		Key:             "object.txt",
	}, localPath, "text/plain")
	if !errors.Is(err, domainstorage.ErrCloudTokenInvalid) {
		t.Fatalf("expected ErrCloudTokenInvalid, got %v", err)
	}
}
