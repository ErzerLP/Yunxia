package config

import (
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
