package storage

import "testing"

func TestBuildPikPakConfigSecretRetentionAndPublicMask(t *testing.T) {
	cfg, err := BuildPikPakConfig(map[string]any{
		"root_folder_id":     " root-id ",
		"platform":           "web",
		"disable_media_link": true,
		"cache_ttl_seconds":  float64(120),
		"download_strategy":  "redirect",
	}, map[string]any{
		"username":      "user@example.com",
		"password":      "password-value",
		"refresh_token": "refresh-1",
		"captcha_token": "captcha-1",
	}, nil)
	if err != nil {
		t.Fatalf("BuildPikPakConfig() error = %v", err)
	}
	if cfg.RootFolderID != "root-id" || cfg.Platform != "web" || cfg.CacheTTLSeconds != 120 {
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

	cleared, err := BuildPikPakConfig(updated.PublicMap(), map[string]any{"password": nil, "captcha_token": nil}, &updated)
	if err != nil {
		t.Fatalf("BuildPikPakConfig(clear) error = %v", err)
	}
	if cleared.Password != "" || cleared.CaptchaToken != "" || cleared.RefreshToken == "" {
		t.Fatalf("expected explicit null to clear selected secrets while keeping refresh token, got %+v", cleared)
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
