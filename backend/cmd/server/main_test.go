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
