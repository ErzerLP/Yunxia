package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAppliesDefaultsAndEnvOverrides(t *testing.T) {
	t.Setenv("YUNXIA_SERVER_PORT", "9090")
	t.Setenv("YUNXIA_DATABASE_DSN", "./test.db")
	t.Setenv("YUNXIA_STORAGE_DATA_DIR", "./data/storage")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Database.DSN != "./test.db" {
		t.Fatalf("expected dsn override, got %q", cfg.Database.DSN)
	}
	if cfg.JWT.AccessTokenExpire != 15*time.Minute {
		t.Fatalf("expected default access token ttl 15m, got %s", cfg.JWT.AccessTokenExpire)
	}
	if cfg.Storage.DefaultChunkSize != 5*1024*1024 {
		t.Fatalf("expected default chunk size 5MB, got %d", cfg.Storage.DefaultChunkSize)
	}
	if cfg.Storage.DataDir != "./data/storage" {
		t.Fatalf("expected storage data dir override, got %q", cfg.Storage.DataDir)
	}
}

func TestLoadIncludesLoggingDefaults(t *testing.T) {
	t.Setenv("YUNXIA_LOGGING_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Logging.Level != "debug" {
		t.Fatalf("expected logging level override, got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Fatalf("expected default logging format json, got %q", cfg.Logging.Format)
	}
	if !cfg.Logging.AccessLogEnabled {
		t.Fatalf("expected access log enabled by default")
	}
}

func TestLoadReadsAria2DownloadDir(t *testing.T) {
	t.Setenv("YUNXIA_ARIA2_DOWNLOAD_DIR", "/downloads")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Aria2.DownloadDir != "/downloads" {
		t.Fatalf("expected aria2 download dir /downloads, got %q", cfg.Aria2.DownloadDir)
	}
}

func TestLoadUsesPasswordlessQBittorrentDefaultsForComposeSidecar(t *testing.T) {
	t.Setenv("YUNXIA_QBITTORRENT_ENABLED", "true")
	t.Setenv("YUNXIA_QBITTORRENT_API_URL", "")
	t.Setenv("YUNXIA_QBITTORRENT_USERNAME", "")
	t.Setenv("YUNXIA_QBITTORRENT_PASSWORD", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.QBittorrent.Enabled {
		t.Fatalf("expected qbittorrent to be enabled by env override")
	}
	if cfg.QBittorrent.APIURL != "http://qbittorrent:8080" {
		t.Fatalf("expected default qbittorrent api url, got %q", cfg.QBittorrent.APIURL)
	}
	if cfg.QBittorrent.Username != "" || cfg.QBittorrent.Password != "" {
		t.Fatalf("expected passwordless sidecar defaults, got username=%q password=%q", cfg.QBittorrent.Username, cfg.QBittorrent.Password)
	}
}

func TestLoadAllowsExplicitQBittorrentCredentials(t *testing.T) {
	t.Setenv("YUNXIA_QBITTORRENT_USERNAME", "admin")
	t.Setenv("YUNXIA_QBITTORRENT_PASSWORD", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.QBittorrent.Username != "admin" {
		t.Fatalf("expected explicit username override, got %q", cfg.QBittorrent.Username)
	}
	if cfg.QBittorrent.Password != "secret" {
		t.Fatalf("expected explicit password override, got %q", cfg.QBittorrent.Password)
	}
}

func TestDockerComposeQBittorrentDefaultsMatchBackendSidecarAuth(t *testing.T) {
	root := findRepoRoot(t)
	composeBytes, err := os.ReadFile(filepath.Join(root, "docker-compose.backend.yml"))
	if err != nil {
		t.Fatalf("ReadFile(compose) error = %v", err)
	}
	entrypointBytes, err := os.ReadFile(filepath.Join(root, "backend", "docker", "qbittorrent.entrypoint.sh"))
	if err != nil {
		t.Fatalf("ReadFile(entrypoint) error = %v", err)
	}
	compose := string(composeBytes)
	entrypoint := string(entrypointBytes)

	for _, want := range []string{
		`YUNXIA_QBITTORRENT_API_URL: "${YUNXIA_QBITTORRENT_API_URL:-http://qbittorrent:8080}"`,
		`YUNXIA_QBITTORRENT_USERNAME: "${YUNXIA_QBITTORRENT_USERNAME:-}"`,
		`YUNXIA_QBITTORRENT_PASSWORD: "${YUNXIA_QBITTORRENT_PASSWORD:-}"`,
		`QBITTORRENT_WEBUI_PORT: "8080"`,
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("docker-compose.backend.yml missing %q", want)
		}
	}
	for _, want := range []string{
		`WebUI\\AuthSubnetWhitelist=0.0.0.0/0`,
		`WebUI\\AuthSubnetWhitelistEnabled=true`,
		`WebUI\\Port=${WEBUI_PORT}`,
	} {
		if !strings.Contains(entrypoint, want) {
			t.Fatalf("qbittorrent.entrypoint.sh missing %q", want)
		}
	}

	t.Setenv("YUNXIA_QBITTORRENT_ENABLED", "true")
	t.Setenv("YUNXIA_QBITTORRENT_API_URL", "")
	t.Setenv("YUNXIA_QBITTORRENT_USERNAME", "")
	t.Setenv("YUNXIA_QBITTORRENT_PASSWORD", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.QBittorrent.APIURL != "http://qbittorrent:8080" {
		t.Fatalf("backend default api url does not match compose sidecar, got %q", cfg.QBittorrent.APIURL)
	}
	if cfg.QBittorrent.Username != "" || cfg.QBittorrent.Password != "" {
		t.Fatalf("backend should skip login for compose sidecar, got username=%q password=%q", cfg.QBittorrent.Username, cfg.QBittorrent.Password)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.backend.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", dir)
		}
		dir = parent
	}
}
