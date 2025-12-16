package config

import "github.com/yungtweek/llm-simulator/internal/logger"

func ApplyPresetOverrides(cfg *Config) {
	logger.Log.Infow("[config] apply profile overrides", "profile", cfg.Preset)
	switch cfg.Preset {
	case "openai":
		// OpenAI-like (general): typical TTFT, moderate throughput, smooth streaming
		cfg.TTFTMinMs = 120
		cfg.TTFTMaxMs = 800
		cfg.TokensPerSec = 35
		cfg.ChunkSize = 16
		cfg.StreamDelayMinMs = 8
		cfg.StreamDelayMaxMs = 45
		cfg.StrictTokenMode = true
		cfg.MaxOutputChars = 12288

	case "vllm":
		// vLLM-like: fast TTFT, high throughput, chunky streaming
		cfg.TTFTMinMs = 30
		cfg.TTFTMaxMs = 200
		cfg.TokensPerSec = 90
		cfg.ChunkSize = 48
		cfg.StreamDelayMinMs = 0
		cfg.StreamDelayMaxMs = 15
		cfg.StrictTokenMode = true
		cfg.MaxOutputChars = 16384

	case "hybrid":
		// Hybrid: balanced, most realistic for production chat
		cfg.TTFTMinMs = 120
		cfg.TTFTMaxMs = 700
		cfg.TokensPerSec = 35
		cfg.ChunkSize = 16
		cfg.StreamDelayMinMs = 8
		cfg.StreamDelayMaxMs = 50
		cfg.StrictTokenMode = true
		cfg.MaxOutputChars = 12288
	}
}
