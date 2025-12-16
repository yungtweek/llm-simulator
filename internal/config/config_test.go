package config

import "testing"

func TestLoadConfigDefaults(t *testing.T) {
	envs := []string{
		"PORT",
		"BASE_DELAY_MS",
		"JITTER_MS",
		"PER_TOKEN_DELAY_MS",
		"ERROR_RATE",
		"ERROR_MODE",
		"DEFAULT_TOKENS",
		"CHUNK_SIZE",
		"STREAM_DELAY_MIN_MS",
		"STREAM_DELAY_MAX_MS",
	}
	for _, k := range envs {
		t.Setenv(k, "")
	}

	cfg := LoadConfig()

	if cfg.Port != 8787 || cfg.BaseDelayMs != 0 || cfg.JitterMs != 0 || cfg.PerTokenDelayMs != 0 {
		t.Fatalf("unexpected base/jitter/per-token: %+v", cfg)
	}
	if cfg.ErrorRate != 0 || cfg.ErrorMode != "mixed" {
		t.Fatalf("unexpected error config: %+v", cfg)
	}
	if cfg.DefaultTokens != 128 || cfg.ChunkSize != 12 {
		t.Fatalf("unexpected token defaults: %+v", cfg)
	}
	if cfg.StreamDelayMinMs != 0 || cfg.StreamDelayMaxMs != 0 {
		t.Fatalf("unexpected stream delay defaults: %+v", cfg)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("BASE_DELAY_MS", "1")
	t.Setenv("JITTER_MS", "2")
	t.Setenv("PER_TOKEN_DELAY_MS", "3")
	t.Setenv("ERROR_RATE", "0.5")
	t.Setenv("ERROR_MODE", "500")
	t.Setenv("DEFAULT_TOKENS", "42")
	t.Setenv("CHUNK_SIZE", "99")
	t.Setenv("STREAM_DELAY_MIN_MS", "5")
	t.Setenv("STREAM_DELAY_MAX_MS", "7")

	cfg := LoadConfig()

	if cfg.Port != 9999 || cfg.BaseDelayMs != 1 || cfg.JitterMs != 2 || cfg.PerTokenDelayMs != 3 {
		t.Fatalf("overrides not applied to delays: %+v", cfg)
	}
	if cfg.ErrorRate != 0.5 || cfg.ErrorMode != "500" {
		t.Fatalf("overrides not applied to error config: %+v", cfg)
	}
	if cfg.DefaultTokens != 42 || cfg.ChunkSize != 99 {
		t.Fatalf("overrides not applied to token config: %+v", cfg)
	}
	if cfg.StreamDelayMinMs != 5 || cfg.StreamDelayMaxMs != 7 {
		t.Fatalf("overrides not applied to stream delays: %+v", cfg)
	}
}
