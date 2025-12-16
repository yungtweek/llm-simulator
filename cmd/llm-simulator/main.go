package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yungtweek/llm-simulator/internal/logger"

	"github.com/yungtweek/llm-simulator/internal/config"
	"github.com/yungtweek/llm-simulator/internal/grpc"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg := config.LoadConfig()
	config.ApplyPresetOverrides(&cfg)

	logger.Init(cfg.Profile)
	defer logger.Sync()

	addr := fmt.Sprintf(":%d", cfg.Port)
	logger.Log.Infow(
		"starting gRPC server",
		"addr", addr,
		"profile", cfg.Preset,
		"baseDelayMs", cfg.BaseDelayMs,
		"jitterMs", cfg.JitterMs,
		"perTokenDelayMs", cfg.PerTokenDelayMs,
		"ttftMinMs", cfg.TTFTMinMs,
		"ttftMaxMs", cfg.TTFTMaxMs,
		"tokensPerSec", cfg.TokensPerSec,
		"errorRate", cfg.ErrorRate,
		"errorMode", cfg.ErrorMode,
		"chunkSize", cfg.ChunkSize,
		"streamDelayMinMs", cfg.StreamDelayMinMs,
		"streamDelayMaxMs", cfg.StreamDelayMaxMs,
		"debugOutputChars", cfg.DebugOutputChars,
		"maxOutputChars", cfg.MaxOutputChars,
		"strictTokenMode", cfg.StrictTokenMode,
	)

	svc := grpc.NewMockLlmService(cfg)
	srv := grpc.NewGRPCServer(addr, svc)

	// Handle SIGINT/SIGTERM for a clean shutdown in local dev / docker.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Log.Info("[llm-simulator] shutting down...")
		srv.GracefulStop()
	}()

	if err := srv.Run(); err != nil {
		logger.Log.Fatalw("[llm-simulator] server error", "err", err)
	}
}
