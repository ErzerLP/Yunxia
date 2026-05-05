package storage

import (
	"crypto/md5"
	"encoding/hex"
	"testing"
)

func TestBuildPikPakConfigSecretRetentionAndPublicMask(t *testing.T) {
	cfg, err := BuildPikPakConfig(map[string]any{
		"root_folder_id":     " root-id ",
		"platform":           "web",
		"disable_media_link": true,
		"cache_ttl_seconds":  float64(120),
		"download_strategy":  "redirect",
		"proxy_url":          " http://127.0.0.1:7890 ",
	}, map[string]any{
		"username":      "user@example.com",
		"password":      "password-value",
		"refresh_token": "refresh-1",
		"captcha_token": "captcha-1",
	}, nil)
	if err != nil {
		t.Fatalf("BuildPikPakConfig() error = %v", err)
	}
	if cfg.RootFolderID != "root-id" || cfg.Platform != "web" || cfg.CacheTTLSeconds != 120 || cfg.ProxyURL != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected public config = %+v", cfg)
	}
	if cfg.DeviceID == "" {
		t.Fatalf("expected generated device_id")
	}
	public := cfg.PublicMap()
	if _, exists := public["username"]; exists {
		t.Fatalf("PublicMap leaked username: %+v", public)
	}
	masks := cfg.SecretMasks()
	if !masks["username"].Configured || masks["username"].Masked != "user****" {
		t.Fatalf("unexpected username mask = %+v", masks["username"])
	}
	if !masks["password"].Configured || masks["password"].Masked != "******" {
		t.Fatalf("unexpected password mask = %+v", masks["password"])
	}

	updated, err := BuildPikPakConfig(map[string]any{
		"root_folder_id":     "",
		"platform":           "pc",
		"disable_media_link": false,
		"cache_ttl_seconds":  300,
		"download_strategy":  "redirect",
	}, nil, &cfg)
	if err != nil {
		t.Fatalf("BuildPikPakConfig(update) error = %v", err)
	}
	if updated.Username != cfg.Username || updated.Password != cfg.Password || updated.RefreshToken != cfg.RefreshToken || updated.CaptchaToken != cfg.CaptchaToken {
		t.Fatalf("expected update to retain secrets, got %+v", updated)
	}
	if updated.Platform != "pc" || updated.DisableMediaLink {
		t.Fatalf("expected public config update, got %+v", updated)
	}
	if updated.ProxyURL != cfg.ProxyURL {
		t.Fatalf("expected omitted proxy_url to retain existing proxy config, got %+v", updated)
	}

	proxyCleared, err := BuildPikPakConfig(map[string]any{
		"root_folder_id":     "",
		"platform":           "pc",
		"disable_media_link": false,
		"cache_ttl_seconds":  300,
		"download_strategy":  "redirect",
		"proxy_url":          "",
	}, nil, &updated)
	if err != nil {
		t.Fatalf("BuildPikPakConfig(clear proxy) error = %v", err)
	}
	if proxyCleared.ProxyURL != "" {
		t.Fatalf("expected explicit empty proxy_url to clear proxy config, got %+v", proxyCleared)
	}

	cleared, err := BuildPikPakConfig(updated.PublicMap(), map[string]any{"password": nil, "captcha_token": nil}, &updated)
	if err != nil {
		t.Fatalf("BuildPikPakConfig(clear) error = %v", err)
	}
	if cleared.Password != "" || cleared.CaptchaToken != "" || cleared.RefreshToken == "" {
		t.Fatalf("expected explicit null to clear selected secrets while keeping refresh token, got %+v", cleared)
	}
}

func TestBuildPikPakConfigPreservesPasswordWhitespaceForAuth(t *testing.T) {
	cfg, err := BuildPikPakConfig(nil, map[string]any{
		"username": "user@example.com",
		"password": " pass-with-edge-space ",
	}, nil)
	if err != nil {
		t.Fatalf("BuildPikPakConfig() error = %v", err)
	}
	if cfg.Password != " pass-with-edge-space " {
		t.Fatalf("password must be preserved exactly for provider auth, got %q", cfg.Password)
	}
	sum := md5.Sum([]byte("user@example.com" + " pass-with-edge-space "))
	expectedDeviceID := hex.EncodeToString(sum[:])
	if cfg.DeviceID != expectedDeviceID {
		t.Fatalf("device_id should be based on exact password, got %q want %q", cfg.DeviceID, expectedDeviceID)
	}

	refreshOnly, err := BuildPikPakConfig(nil, map[string]any{
		"refresh_token": "refresh-token",
	}, nil)
	if err != nil {
		t.Fatalf("BuildPikPakConfig(refresh only) error = %v", err)
	}
	if refreshOnly.DeviceID == "" {
		t.Fatalf("refresh-token-only config should still get a stable provider device_id")
	}
}

func TestParsePikPakConfigJSONRejectsInvalidPlatform(t *testing.T) {
	_, err := BuildPikPakConfig(map[string]any{
		"platform": "ios",
	}, map[string]any{"refresh_token": "refresh"}, nil)
	if err == nil {
		t.Fatalf("expected invalid platform error")
	}
}

func TestBuildPikPakConfigRejectsUnsafeProxyURL(t *testing.T) {
	cases := []string{
		"127.0.0.1:7890",
		"socks5://127.0.0.1:7890",
		"http://user:pass@127.0.0.1:7890",
		"http://127.0.0.1:7890?token=secret",
		"http://127.0.0.1:7890#frag",
	}
	for _, proxyURL := range cases {
		t.Run(proxyURL, func(t *testing.T) {
			_, err := BuildPikPakConfig(map[string]any{
				"proxy_url": proxyURL,
			}, map[string]any{"refresh_token": "refresh"}, nil)
			if err == nil {
				t.Fatalf("expected invalid proxy_url %q to be rejected", proxyURL)
			}
		})
	}
}
