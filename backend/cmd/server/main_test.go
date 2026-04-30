package main

import (
	"testing"

	appcfg "yunxia/internal/infrastructure/config"
)

func TestTaskStagingRootUsesSharedAria2DownloadDir(t *testing.T) {
	cfg := appcfg.Config{}
	cfg.Storage.TempDir = "/app/data/temp"
	cfg.Aria2.DownloadDir = "/downloads"

	got := taskStagingRoot(cfg)
	if got != "/downloads/staging" {
		t.Fatalf("expected /downloads/staging, got %q", got)
	}
}

func TestTaskStagingRootKeepsAria2AsGlobalDefaultWhenQBitAlsoConfigured(t *testing.T) {
	cfg := appcfg.Config{}
	cfg.Storage.TempDir = "/app/data/temp"
	cfg.Aria2.DownloadDir = "/aria2-downloads"
	cfg.QBittorrent.DownloadDir = "/qbit-downloads"

	got := taskStagingRoot(cfg)
	if got != "/aria2-downloads/staging" {
		t.Fatalf("expected aria2 staging root, got %q", got)
	}
}

func TestDownloadStagingRootReturnsEmptyForBlankDir(t *testing.T) {
	if got := downloadStagingRoot(""); got != "" {
		t.Fatalf("expected empty staging root, got %q", got)
	}
}
