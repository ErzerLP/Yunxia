package downloader

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	appsvc "yunxia/internal/application/service"
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
			mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
			if err != nil {
				t.Fatalf("mime.ParseMediaType() error = %v", err)
			}
			if mediaType != "multipart/form-data" {
				t.Fatalf("expected multipart/form-data, got %q", mediaType)
			}
			if err := request.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm() error = %v", err)
			}
			captured = request.MultipartForm.Value
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

func TestQBittorrentClientAddTorrentURLUploadsDownloadedTorrentFile(t *testing.T) {
	var capturedValues url.Values
	var capturedFileName string
	var capturedFileBody string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v2/auth/login":
			_, _ = writer.Write([]byte("Ok."))
		case "/files/show.torrent":
			_, _ = writer.Write([]byte("d8:announce13:http://tracker4:infod4:name4:showee"))
		case "/api/v2/torrents/add":
			mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
			if err != nil {
				t.Fatalf("mime.ParseMediaType() error = %v", err)
			}
			if mediaType != "multipart/form-data" {
				t.Fatalf("expected multipart/form-data, got %q", mediaType)
			}
			if err := request.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm() error = %v", err)
			}
			capturedValues = request.MultipartForm.Value
			files := request.MultipartForm.File["torrents"]
			if len(files) != 1 {
				t.Fatalf("expected one torrents file, got %d", len(files))
			}
			capturedFileName = files[0].Filename
			file, err := files[0].Open()
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer file.Close()
			body, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			capturedFileBody = string(body)
			_, _ = writer.Write([]byte("Ok."))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client := NewQBittorrentClient(server.URL, "admin", "adminadmin")
	externalID, err := client.AddURI(context.Background(), server.URL+"/files/show.torrent?passkey=abc", "/downloads/staging/task_1")
	if err != nil {
		t.Fatalf("AddURI() error = %v", err)
	}
	if capturedValues.Get("urls") != "" {
		t.Fatalf("torrent URL should be uploaded as file, got urls=%q", capturedValues.Get("urls"))
	}
	if capturedValues.Get("savepath") != "/downloads/staging/task_1" {
		t.Fatalf("unexpected savepath = %q", capturedValues.Get("savepath"))
	}
	if capturedValues.Get("tags") != externalID {
		t.Fatalf("expected tags to match external id, got %q / %q", capturedValues.Get("tags"), externalID)
	}
	if capturedFileName != "show.torrent" {
		t.Fatalf("unexpected uploaded filename %q", capturedFileName)
	}
	if capturedFileBody != "d8:announce13:http://tracker4:infod4:name4:showee" {
		t.Fatalf("unexpected uploaded body %q", capturedFileBody)
	}
}

func TestQBittorrentClientAddTorrentURLFetchFailureDoesNotPostAdd(t *testing.T) {
	addCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/files/missing.torrent":
			http.Error(writer, "missing", http.StatusNotFound)
		case "/api/v2/torrents/add":
			addCalled = true
			t.Fatalf("torrent add should not be called when backend fetch fails")
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client := NewQBittorrentClient(server.URL, "admin", "adminadmin")
	_, err := client.AddURI(context.Background(), server.URL+"/files/missing.torrent", "/downloads/staging/task_1")
	if err == nil || !strings.Contains(err.Error(), "torrent fetch status 404") {
		t.Fatalf("expected torrent fetch status error, got %v", err)
	}
	if addCalled {
		t.Fatalf("torrent add should not be called")
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

func TestQBittorrentClientLoginUsesWebAPIFormFields(t *testing.T) {
	var loginCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v2/auth/login":
			loginCalled = true
			if request.Method != http.MethodPost {
				t.Fatalf("expected POST login, got %s", request.Method)
			}
			if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Fatalf("unexpected content type %q", request.Header.Get("Content-Type"))
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if request.Form.Get("username") != "admin" {
				t.Fatalf("expected username form field, got %q", request.Form.Get("username"))
			}
			if request.Form.Get("password") != "secret" {
				t.Fatalf("expected password form field, got %q", request.Form.Get("password"))
			}
			if request.Form.Get("user") != "" || request.Form.Get("pass") != "" {
				t.Fatalf("login form used unexpected aliases: %v", request.Form)
			}
			_, _ = writer.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = writer.Write([]byte("v5.0.0"))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client := NewQBittorrentClient(server.URL, "admin", "secret")
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if !loginCalled {
		t.Fatalf("expected login to be called")
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
	err := client.Health(context.Background())
	if err == nil {
		t.Fatalf("expected login failure")
	}
	if !errors.Is(err, appsvc.ErrDownloaderAuthFailed) {
		t.Fatalf("expected ErrDownloaderAuthFailed, got %v", err)
	}
}

func TestQBittorrentClientLoginUnauthorizedReturnsDiagnosticStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v2/auth/login" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		http.Error(writer, "Unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewQBittorrentClient(server.URL, "admin", "bad-password")
	err := client.Health(context.Background())
	if err == nil {
		t.Fatalf("expected login failure")
	}
	if !strings.Contains(err.Error(), "qbittorrent login status 401") {
		t.Fatalf("expected diagnostic 401 status, got %v", err)
	}
	if !errors.Is(err, appsvc.ErrDownloaderAuthFailed) {
		t.Fatalf("expected ErrDownloaderAuthFailed, got %v", err)
	}
}

func TestQBittorrentClientHealthUnauthorizedReturnsDownloaderAuthFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v2/app/version" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		http.Error(writer, "Unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewQBittorrentClient(server.URL, "", "")
	err := client.Health(context.Background())
	if err == nil {
		t.Fatalf("expected health failure")
	}
	if !strings.Contains(err.Error(), "qbittorrent health status 401") {
		t.Fatalf("expected diagnostic health 401 status, got %v", err)
	}
	if !errors.Is(err, appsvc.ErrDownloaderAuthFailed) {
		t.Fatalf("expected ErrDownloaderAuthFailed, got %v", err)
	}
}

func TestQBittorrentClientAddURIUnauthorizedReturnsDownloaderAuthFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v2/torrents/add" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		http.Error(writer, "Unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewQBittorrentClient(server.URL, "", "")
	_, err := client.AddURI(context.Background(), "magnet:?xt=urn:btih:abcdef", "/downloads/staging/task_1")
	if err == nil {
		t.Fatalf("expected add failure")
	}
	if !strings.Contains(err.Error(), "qbittorrent /api/v2/torrents/add status 401") {
		t.Fatalf("expected diagnostic add 401 status, got %v", err)
	}
	if !errors.Is(err, appsvc.ErrDownloaderAuthFailed) {
		t.Fatalf("expected ErrDownloaderAuthFailed, got %v", err)
	}
}

func TestQBittorrentAuthStatusErrorDoesNotExposeResponseBody(t *testing.T) {
	err := qbitStatusError("qbittorrent health", http.StatusUnauthorized, "password=secret-token")
	if !errors.Is(err, appsvc.ErrDownloaderAuthFailed) {
		t.Fatalf("expected ErrDownloaderAuthFailed, got %v", err)
	}
	if !strings.Contains(err.Error(), "qbittorrent health status 401") {
		t.Fatalf("expected diagnostic status, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "password=") {
		t.Fatalf("auth status error leaked response body: %v", err)
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

func TestQBittorrentClientTellStatusKeepsMissingTagPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v2/auth/login":
			_, _ = writer.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = writer.Write([]byte("[]"))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client := NewQBittorrentClient(server.URL, "admin", "adminadmin")
	status, err := client.TellStatus(context.Background(), "yunxia-task-missing")
	if err != nil {
		t.Fatalf("TellStatus() error = %v", err)
	}
	if status.Status != "pending" {
		t.Fatalf("expected pending for temporarily invisible tag, got %q", status.Status)
	}
	if status.ErrorMessage == nil || !strings.Contains(*status.ErrorMessage, "not visible yet") {
		t.Fatalf("expected diagnostic error message, got %+v", status.ErrorMessage)
	}
}

func TestMapQBitTorrentStatusSetsFailedErrorMessage(t *testing.T) {
	status := mapQBitTorrentStatus(qbitTorrentInfo{
		Name:  "show.mkv",
		State: "missingFiles",
	})
	if status.Status != "failed" {
		t.Fatalf("expected failed, got %q", status.Status)
	}
	if status.ErrorMessage == nil || !strings.Contains(*status.ErrorMessage, "missingFiles") {
		t.Fatalf("expected failed state error message, got %+v", status.ErrorMessage)
	}
}
