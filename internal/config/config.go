package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port             int
	Profile          string
	Preset           string // openai|vllm|hybrid (controls default behavior presets)
	BaseDelayMs      int
	JitterMs         int
	PerTokenDelayMs  int
	ErrorRate        float64
	ErrorMode        string // mixed|429|500
	DefaultTokens    int
	ChunkSize        int
	StreamDelayMinMs int
	StreamDelayMaxMs int
	EchoPrompt       bool
	Randomize        bool // enable/disable output-length & stream-shape randomization

	// LLM-like timing
	TTFTMinMs    int // time-to-first-token min
	TTFTMaxMs    int // time-to-first-token max
	TokensPerSec int // streaming speed (approx)

	// Output sizing
	DebugOutputChars int  // fixed output size for debugging
	MaxOutputChars   int  // upper bound when using token-based sizing
	StrictTokenMode  bool // if true, size output based on max_tokens
}

func getEnvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
func getEnvFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
func getEnvStr(k string, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "y", "on":
			return true
		case "0", "false", "no", "n", "off":
			return false
		}
	}
	return def
}

func LoadConfig() Config {
	return Config{
		Port:             getEnvInt("PORT", 8787),
		Profile:          getEnvStr("PROFILE", "default"),
		Preset:           strings.ToLower(getEnvStr("PRESET", "openai")),
		BaseDelayMs:      getEnvInt("BASE_DELAY_MS", 0),
		JitterMs:         getEnvInt("JITTER_MS", 0),
		PerTokenDelayMs:  getEnvInt("PER_TOKEN_DELAY_MS", 0),
		ErrorRate:        getEnvFloat("ERROR_RATE", 0),
		ErrorMode:        strings.ToLower(getEnvStr("ERROR_MODE", "mixed")),
		DefaultTokens:    getEnvInt("DEFAULT_TOKENS", 128),
		ChunkSize:        getEnvInt("CHUNK_SIZE", 12),
		StreamDelayMinMs: getEnvInt("STREAM_DELAY_MIN_MS", 0),
		StreamDelayMaxMs: getEnvInt("STREAM_DELAY_MAX_MS", 0),
		EchoPrompt:       getBool("ECHO_PROMPT", false),
		Randomize:        getBool("RANDOMIZE", false),

		// LLM-like timing
		TTFTMinMs:    getEnvInt("TTFT_MIN_MS", 0),
		TTFTMaxMs:    getEnvInt("TTFT_MAX_MS", 0),
		TokensPerSec: getEnvInt("TOKENS_PER_SEC", 120),

		// Output sizing
		DebugOutputChars: getEnvInt("DEBUG_OUTPUT_CHARS", 0),
		MaxOutputChars:   getEnvInt("MAX_OUTPUT_CHARS", 16384),
		StrictTokenMode:  getBool("STRICT_TOKEN_MODE", true),
	}
}
