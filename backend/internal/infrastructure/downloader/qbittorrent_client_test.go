package downloader

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestQBittorrentClientAddURISendsSavePathAndTag(t *testing.T) {
	var loginCalled bool
	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v2/auth/login":
			loginCalled = true
			_, _ = writer.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("io.ReadAll() error = %v", err)
			}
			captured, err = url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("url.ParseQuery() error = %v", err)
			}
			_, _ = writer.Write([]byte("Ok."))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client := NewQBittorrentClient(server.URL, "admin", "adminadmin")
	externalID, err := client.AddURI(context.Background(), "magnet:?xt=urn:btih:abcdef", "/downloads/staging/task_1")
	if err != nil {
		t.Fatalf("AddURI() error = %v", err)
	}
	if !loginCalled {
		t.Fatalf("expected login to be called")
	}
	if !strings.HasPrefix(externalID, "yunxia-task-") {
		t.Fatalf("expected yunxia task tag, got %q", externalID)
	}
	if captured.Get("urls") != "magnet:?xt=urn:btih:abcdef" {
		t.Fatalf("unexpected urls = %q", captured.Get("urls"))
	}
	if captured.Get("savepath") != "/downloads/staging/task_1" {
		t.Fatalf("unexpected savepath = %q", captured.Get("savepath"))
	}
	if captured.Get("tags") != externalID {
		t.Fatalf("expected tags to match external id, got %q / %q", captured.Get("tags"), externalID)
	}
}

func TestQBittorrentClientSkipsLoginWhenCredentialsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/auth/login" {
			t.Fatalf("login should not be called when credentials are empty")
		}
		if request.URL.Path != "/api/v2/app/version" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		_, _ = writer.Write([]byte("v5.0.0"))
	}))
	defer server.Close()

	client := NewQBittorrentClient(server.URL, "", "")
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestQBittorrentClientLoginFailureReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v2/auth/login" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		_, _ = writer.Write([]byte("Fails."))
	}))
	defer server.Close()

	client := NewQBittorrentClient(server.URL, "admin", "bad-password")
	if err := client.Health(context.Background()); err == nil {
		t.Fatalf("expected login failure")
	}
}

func TestMapQBitTorrentStatusMapsCompletedTorrent(t *testing.T) {
	status := mapQBitTorrentStatus(qbitTorrentInfo{
		Name:       "show.mkv",
		State:      "stalledUP",
		Progress:   1,
		Downloaded: 1024,
		TotalSize:  1024,
		AmountLeft: 0,
		DLSpeed:    2048,
		ETA:        10,
	})
	if status.Status != "completed" {
		t.Fatalf("expected completed, got %q", status.Status)
	}
	if status.DisplayName != "show.mkv" {
		t.Fatalf("unexpected display name %q", status.DisplayName)
	}
}
