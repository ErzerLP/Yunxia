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
	t.Setenv("YUNXIA_DATABASE_DSN", "postgres://tester:secret@localhost:5432/yunxia_test?sslmode=disable")
	t.Setenv("YUNXIA_DATABASE_MAX_OPEN_CONNS", "12")
	t.Setenv("YUNXIA_DATABASE_CONN_MAX_LIFETIME", "30m")
	t.Setenv("YUNXIA_STORAGE_DATA_DIR", "./data/storage")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Database.DSN != "postgres://tester:secret@localhost:5432/yunxia_test?sslmode=disable" {
		t.Fatalf("expected dsn override, got %q", cfg.Database.DSN)
	}
	if !cfg.Database.AutoMigrate {
		t.Fatalf("expected auto migrate enabled by default")
	}
	if cfg.Database.MaxOpenConns != 12 {
		t.Fatalf("expected max open conns override 12, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 5 {
		t.Fatalf("expected default max idle conns 5, got %d", cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetime != 30*time.Minute {
		t.Fatalf("expected conn max lifetime override 30m, got %s", cfg.Database.ConnMaxLifetime)
	}
	if cfg.Database.ConnMaxIdleTime != 10*time.Minute {
		t.Fatalf("expected default conn max idle time 10m, got %s", cfg.Database.ConnMaxIdleTime)
	}
	if cfg.Database.SlowThreshold != 500*time.Millisecond {
		t.Fatalf("expected default slow threshold 500ms, got %s", cfg.Database.SlowThreshold)
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

func TestLoadUsesPostgresDatabaseDefaults(t *testing.T) {
	t.Setenv("YUNXIA_DATABASE_DSN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !strings.HasPrefix(cfg.Database.DSN, "postgres://") {
		t.Fatalf("expected PostgreSQL default DSN, got %q", cfg.Database.DSN)
	}
	if strings.Contains(strings.ToLower(cfg.Database.DSN), "sqlite") || strings.Contains(cfg.Database.DSN, "database.db") {
		t.Fatalf("default DSN should not point to SQLite/file DB, got %q", cfg.Database.DSN)
	}
	if !cfg.Database.AutoMigrate {
		t.Fatalf("expected auto migrate enabled by default")
	}
	if cfg.Database.MaxOpenConns != 25 || cfg.Database.MaxIdleConns != 5 {
		t.Fatalf("unexpected pool defaults: max_open=%d max_idle=%d", cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns)
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

func TestDockerComposePostgresDefaultsMatchBackendDatabaseConfig(t *testing.T) {
	root := findRepoRoot(t)
	composeBytes, err := os.ReadFile(filepath.Join(root, "docker-compose.backend.yml"))
	if err != nil {
		t.Fatalf("ReadFile(compose) error = %v", err)
	}
	compose := string(composeBytes)

	for _, want := range []string{
		`postgres:`,
		`image: "${YUNXIA_DOCKER_POSTGRES_IMAGE:-postgres:16-alpine}"`,
		`POSTGRES_DB: "${YUNXIA_POSTGRES_DB:-yunxia}"`,
		`POSTGRES_USER: "${YUNXIA_POSTGRES_USER:-yunxia}"`,
		`POSTGRES_PASSWORD: "${YUNXIA_POSTGRES_PASSWORD:-yunxia}"`,
		`- postgres-data:/var/lib/postgresql/data`,
		`pg_isready -U ${YUNXIA_POSTGRES_USER:-yunxia} -d ${YUNXIA_POSTGRES_DB:-yunxia}`,
		`YUNXIA_DATABASE_DSN: "${YUNXIA_DATABASE_DSN:-postgres://${YUNXIA_POSTGRES_USER:-yunxia}:${YUNXIA_POSTGRES_PASSWORD:-yunxia}@postgres:5432/${YUNXIA_POSTGRES_DB:-yunxia}?sslmode=disable&TimeZone=Asia/Shanghai}"`,
		`YUNXIA_DATABASE_AUTO_MIGRATE: "${YUNXIA_DATABASE_AUTO_MIGRATE:-true}"`,
		`YUNXIA_DATABASE_MAX_OPEN_CONNS: "${YUNXIA_DATABASE_MAX_OPEN_CONNS:-25}"`,
		`YUNXIA_DATABASE_MAX_IDLE_CONNS: "${YUNXIA_DATABASE_MAX_IDLE_CONNS:-5}"`,
		`YUNXIA_DATABASE_CONN_MAX_LIFETIME: "${YUNXIA_DATABASE_CONN_MAX_LIFETIME:-1h}"`,
		`YUNXIA_DATABASE_CONN_MAX_IDLE_TIME: "${YUNXIA_DATABASE_CONN_MAX_IDLE_TIME:-10m}"`,
		`YUNXIA_DATABASE_SLOW_THRESHOLD: "${YUNXIA_DATABASE_SLOW_THRESHOLD:-500ms}"`,
		`postgres-data:`,
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("docker-compose.backend.yml missing %q", want)
		}
	}
	if !strings.Contains(compose, "postgres:") ||
		!strings.Contains(compose, "condition: service_healthy") {
		t.Fatalf("backend should depend on PostgreSQL healthcheck before startup")
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
		`set_qbt_conf "Preferences" "WebUI\\AuthSubnetWhitelist" "0.0.0.0/0, ::/0"`,
		`set_qbt_conf "Preferences" "WebUI\\AuthSubnetWhitelistEnabled" "true"`,
		`set_qbt_conf "Preferences" "WebUI\\HostHeaderValidation" "false"`,
		`set_qbt_conf "Preferences" "WebUI\\CSRFProtection" "false"`,
		`set_qbt_conf "Preferences" "WebUI\\SecureCookie" "false"`,
		`set_qbt_conf "Preferences" "WebUI\\Port" "${WEBUI_PORT}"`,
		`WebUI\\AuthSubnetWhitelist=0.0.0.0/0`,
	} {
		if !strings.Contains(entrypoint, want) {
			t.Fatalf("qbittorrent.entrypoint.sh missing %q", want)
		}
	}
	if !strings.Contains(entrypoint, "Patch the internal sidecar API settings on every start") {
		t.Fatalf("qbittorrent.entrypoint.sh should document idempotent existing-volume patching")
	}
	if !strings.Contains(entrypoint, `QBT_KEY="${key}"`) || strings.Contains(entrypoint, `awk -v section=`) {
		t.Fatalf("qbittorrent.entrypoint.sh should pass backslash-containing config keys to awk without -v escape rewriting")
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
