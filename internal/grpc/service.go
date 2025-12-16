package grpc

import (
	"context"
	"errors"
	"github.com/yungtweek/llm-simulator/internal/config"
	"github.com/yungtweek/llm-simulator/internal/logger"
	"github.com/yungtweek/llm-simulator/internal/mock"
	"strings"
	"time"

	llmv1 "github.com/yungtweek/llm-simulator/gen"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// MockLlmService implements llm.v1.LlmService for benchmarking.
//
// It simulates:
//   - base + jitter latency
//   - optional per-token latency
//   - error injection
//   - server-streaming chunked deltas
//
// This is a mock/benchmark tool, so behavior is intentionally deterministic-ish
// and configurable via Config.
type MockLlmService struct {
	llmv1.UnimplementedLlmServiceServer
	cfg config.Config
}

func NewMockLlmService(cfg config.Config) *MockLlmService {
	return &MockLlmService{cfg: cfg}
}

func (s *MockLlmService) ChatCompletion(ctx context.Context, req *llmv1.ChatCompletionRequest) (*llmv1.ChatCompletionResponse, error) {
	start := time.Now()
	logger.Log.Infow("[grpc][ChatCompletion] start", "model", req.GetModel(), "maxTokens", req.GetMaxTokens())

	// Error injection (before any work).
	if shouldFail(s.cfg.ErrorRate) {
		logger.Log.Infow("[grpc][ChatCompletion] injected error", "mode", s.cfg.ErrorMode)
		return nil, status.Error(pickGrpcErrorCode(s.cfg.ErrorMode), "mock error")
	}

	maxTokens := req.GetMaxTokens()
	if maxTokens <= 0 {
		maxTokens = int32(defaultInt(s.cfg.DefaultTokens, 128))
	}

	// Randomize output length in a chat-like distribution (short is common, long is rare).
	effectiveMaxTokens := maxTokens

	// Simulate compute latency.
	prompt := buildPromptForTokens(req)
	if s.cfg.Randomize {
		effectiveMaxTokens = pickTargetTokens(maxTokens, len([]rune(prompt)))
	}
	out := mock.BuildOutput(prompt, int(effectiveMaxTokens), s.cfg.EchoPrompt, s.cfg.StrictTokenMode, s.cfg.DebugOutputChars, s.cfg.MaxOutputChars)

	pt := int32(mock.ApproxTokens(prompt))
	ct := int32(mock.ApproxTokens(out))

	// Simulate total latency (roughly): base+jitter + TTFT + generation time.
	computeMs := s.baseDelayMs() + s.jitterMs() + s.ttftMs()
	// Optional per-token overhead (e.g., server-side processing).
	computeMs += s.perTokenDelayMs(int(ct)) * int(ct)
	// Token generation time from TokensPerSec.
	if tps := s.tokensPerSec(); tps > 0 {
		computeMs += int((ct * 1000) / int32(tps))
	}
	sleepWithContext(ctx, time.Duration(computeMs)*time.Millisecond)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	resp := &llmv1.ChatCompletionResponse{
		OutputText:       out,
		FinishReason:     "stop",
		PromptTokens:     pt,
		CompletionTokens: ct,
		TotalTokens:      pt + ct,
		LatencyMs:        time.Since(start).Milliseconds(),
	}
	logger.Log.Infow("[grpc][ChatCompletion] completed", "latencyMs", resp.LatencyMs, "tokens", resp.TotalTokens)
	return resp, nil
}

func (s *MockLlmService) ChatCompletionStream(req *llmv1.ChatCompletionRequest, stream llmv1.LlmService_ChatCompletionStreamServer) (err error) {
	ctx := stream.Context()
	start := time.Now()
	var peerAddr string
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		peerAddr = p.Addr.String()
	} else {
		peerAddr = "unknown"
	}
	logger.Log.Infow("[grpc][ChatCompletionStream] start", "peer", peerAddr, "model", req.GetModel(), "maxTokens", req.GetMaxTokens())

	defer func() {
		// Log termination exactly once for all outcomes.
		switch {
		case err == nil:
			logger.Log.Infow("[grpc][ChatCompletionStream] done", "peer", peerAddr)
		case errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled:
			logger.Log.Infow("[grpc][ChatCompletionStream] canceled", "peer", peerAddr, "err", err)
		case errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded:
			logger.Log.Warnw("[grpc][ChatCompletionStream] deadline_exceeded", "peer", peerAddr, "err", err)
		default:
			logger.Log.Errorw("[grpc][ChatCompletionStream] error", "peer", peerAddr, "err", err)
		}

		// Best-effort: emit a final failed chunk so workers can finalize state.
		if err != nil {
			reason := err.Error()
			_ = stream.Send(&llmv1.ChatCompletionChunkResponse{
				Type:         "failed",
				Text:         "",
				Index:        0,
				FinishReason: reason,
			})
		}
	}()

	// Error injection (before sending any chunks).
	if shouldFail(s.cfg.ErrorRate) {
		logger.Log.Infow("[grpc][ChatCompletionStream] injected error", "mode", s.cfg.ErrorMode)
		return status.Error(pickGrpcErrorCode(s.cfg.ErrorMode), "mock error")
	}

	maxTokens := req.GetMaxTokens()
	if maxTokens <= 0 {
		maxTokens = int32(defaultInt(s.cfg.DefaultTokens, 128))
	}

	// Randomize output length in a chat-like distribution (short is common, long is rare).
	effectiveMaxTokens := maxTokens

	// Delay before the first token.
	// IMPORTANT: keep this small so clients with short deadlines still receive the first chunk.
	pre := time.Duration(s.baseDelayMs()+s.jitterMs()+s.ttftMs()) * time.Millisecond
	logger.Log.Infow("[grpc][ChatCompletionStream] pre_delay", "peer", peerAddr, "delayMs", pre.Milliseconds())
	if pre > 0 {
		sleepWithContext(ctx, pre)
		logger.Log.Infow("[grpc][ChatCompletionStream] pre_delay_done", "peer", peerAddr)
		if err = ctx.Err(); err != nil {
			logger.Log.Warnw("[grpc][ChatCompletionStream] context error during pre_delay", "peer", peerAddr, "err", err)
			return err
		}
	}

	prompt := buildPromptForTokens(req)
	if s.cfg.Randomize {
		effectiveMaxTokens = pickTargetTokens(maxTokens, len([]rune(prompt)))
	}

	chunkSize := s.chunkSize()
	if chunkSize <= 0 {
		chunkSize = 12
	}
	if s.cfg.Randomize {
		// Randomize chunk size a bit (+/- 33%) to vary stream shape.
		if chunkSize > 1 {
			j := chunkSize / 3
			if j < 1 {
				j = 1
			}
			chunkSize = (chunkSize - j) + mock.RandIntn(j*2+1)
			if chunkSize < 1 {
				chunkSize = 1
			}
		}
	}

	out := mock.BuildOutput(prompt, int(effectiveMaxTokens), s.cfg.EchoPrompt, s.cfg.StrictTokenMode, s.cfg.DebugOutputChars, s.cfg.MaxOutputChars)
	logger.Log.Infow("[grpc][ChatCompletionStream] generated output", "peer", peerAddr, "outputLen", len(out), "chunkSize", chunkSize)

	pt := int32(mock.ApproxTokens(prompt))
	ct := int32(mock.ApproxTokens(out))

	// Stream content deltas.
	loggedFirstChunk := false
	for i := 0; i < len(out); i += chunkSize {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := i + chunkSize
		if end > len(out) {
			end = len(out)
		}
		delta := out[i:end]

		if !loggedFirstChunk {
			logger.Log.Infow("[grpc][ChatCompletionStream] sending first chunk", "peer", peerAddr, "size", len(delta))
			loggedFirstChunk = true
		}

		if err = stream.Send(&llmv1.ChatCompletionChunkResponse{
			Type:  "output_text.delta",
			Text:  delta,
			Index: 0,
		}); err != nil {
			return err
		}

		// Optional chunk pacing.
		s.sleepStreamGap(ctx, delta)
		if err = ctx.Err(); err != nil {
			return err
		}
	}

	// Emit a separate done event (no full text; worker assembles from deltas).
	logger.Log.Infow(
		"[grpc][ChatCompletionStream] sending done chunk",
		"peer", peerAddr,
		"latencyMs", time.Since(start).Milliseconds(),
		"totalTokens", pt+ct,
	)
	if err = stream.Send(&llmv1.ChatCompletionChunkResponse{
		Type:             "output_text.done",
		Text:             "",
		Index:            0,
		FinishReason:     "stop",
		PromptTokens:     pt,
		CompletionTokens: ct,
		TotalTokens:      pt + ct,
		LatencyMs:        time.Since(start).Milliseconds(),
	}); err != nil {
		return err
	}

	return nil
}

// ---- helpers ----

// pickTargetTokens chooses a target token budget that feels like real chat:
// short answers are common, long answers are rare.
// It returns a value in [1, maxTokens]. If maxTokens <= 0, it uses 128.
func pickTargetTokens(maxTokens int32, promptRunes int) int32 {
	if maxTokens <= 0 {
		maxTokens = 128
	}

	// Base probabilities.
	pShort := 0.58
	pNormal := 0.34
	pLong := 0.07
	pMaxed := 0.01

	// Bias slightly toward longer outputs when the prompt is long.
	// (This keeps "tell me everything" prompts from always returning short replies.)
	if promptRunes > 1200 {
		pShort -= 0.10
		pNormal += 0.06
		pLong += 0.03
		pMaxed += 0.01
	} else if promptRunes > 600 {
		pShort -= 0.06
		pNormal += 0.04
		pLong += 0.02
	}

	// Clamp (defensive).
	if pShort < 0.10 {
		pShort = 0.10
	}
	if pMaxed < 0.0 {
		pMaxed = 0.0
	}

	r := mock.RandFloat64()

	// Helper: pick an integer token count from a fractional range of maxTokens.
	pickFrac := func(minF, maxF float64) int32 {
		minT := int32(float64(maxTokens) * minF)
		maxT := int32(float64(maxTokens) * maxF)
		if minT < 1 {
			minT = 1
		}
		if maxT < minT {
			maxT = minT
		}
		if maxT == minT {
			return minT
		}
		return minT + int32(mock.RandIntn(int(maxT-minT+1)))
	}

	switch {
	case r < pShort:
		// 1-3 sentences
		return pickFrac(0.05, 0.22)
	case r < pShort+pNormal:
		// a few short paragraphs
		return pickFrac(0.22, 0.62)
	case r < pShort+pNormal+pLong:
		// long-ish explanation
		return pickFrac(0.62, 0.92)
	default:
		// rare: push to the cap (simulates verbose answers / near-length outputs)
		_ = pMaxed // kept for readability
		return pickFrac(0.92, 1.00)
	}
}

func (s *MockLlmService) baseDelayMs() int {
	return defaultInt(s.cfg.BaseDelayMs, 0)
}

func (s *MockLlmService) jitterMs() int {
	j := defaultInt(s.cfg.JitterMs, 0)
	if j <= 0 {
		return 0
	}
	// rng is expected to be initialized at package scope (see mock.go)
	return mock.RandIntn(j + 1)
}

func (s *MockLlmService) perTokenDelayMs(maxTokens int) int {
	return defaultInt(s.cfg.PerTokenDelayMs, 0)
}

func (s *MockLlmService) chunkSize() int {
	return defaultInt(s.cfg.ChunkSize, 12)
}

func (s *MockLlmService) ttftMs() int {
	min := defaultInt(s.cfg.TTFTMinMs, 0)
	max := defaultInt(s.cfg.TTFTMaxMs, 0)
	if min <= 0 && max <= 0 {
		return 0
	}
	if min <= 0 {
		min = max
	}
	if max <= 0 {
		max = min
	}
	if max < min {
		max = min
	}
	if max == min {
		return min
	}
	return min + mock.RandIntn(max-min+1)
}

func (s *MockLlmService) tokensPerSec() int {
	return defaultInt(s.cfg.TokensPerSec, 0)
}

func (s *MockLlmService) sleepStreamGap(ctx context.Context, delta string) {
	ms := 0

	// Base gap jitter (existing knobs).
	min := defaultInt(s.cfg.StreamDelayMinMs, 0)
	max := defaultInt(s.cfg.StreamDelayMaxMs, 0)
	if max > 0 {
		if max < min {
			max = min
		}
		ms += min
		if max > min {
			ms += mock.RandIntn(max - min + 1)
		}
	}

	// Approx generation pacing from tokens/sec.
	if tps := s.tokensPerSec(); tps > 0 {
		// Rough: 1 token ~= 4 runes.
		toks := mock.ApproxTokens(delta)
		if toks < 1 {
			toks = 1
		}
		msPerTok := 1000 / tps
		if msPerTok < 1 {
			msPerTok = 1
		}
		ms += toks * msPerTok
	}

	// Optional per-token overhead.
	per := defaultInt(s.cfg.PerTokenDelayMs, 0)
	if per > 0 {
		toks := mock.ApproxTokens(delta)
		if toks < 1 {
			toks = 1
		}
		ms += per * toks
	}

	sleepWithContext(ctx, time.Duration(ms)*time.Millisecond)
}

func defaultInt(v int, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func shouldFail(rate float64) bool {
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	return mock.RandFloat64() < rate
}

func pickGrpcErrorCode(mode string) codes.Code {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "429", "resource_exhausted", "rate_limit", "rate limit":
		return codes.ResourceExhausted
	case "500", "internal", "server_error":
		return codes.Internal
	default:
		// mixed
		if mock.RandIntn(2) == 0 {
			return codes.ResourceExhausted
		}
		return codes.Internal
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return
	case <-t.C:
		return
	}
}

func buildPromptForTokens(req *llmv1.ChatCompletionRequest) string {
	var b strings.Builder
	if sp := strings.TrimSpace(req.GetSystemPrompt()); sp != "" {
		b.WriteString("[system]\n")
		b.WriteString(sp)
		b.WriteString("\n\n")
	}

	// Prior context messages.
	for _, m := range req.GetContext() {
		role := strings.TrimSpace(m.GetRole())
		content := strings.TrimSpace(m.GetContent())
		if role == "" && content == "" {
			continue
		}
		b.WriteString("[")
		if role == "" {
			role = "unknown"
		}
		b.WriteString(role)
		b.WriteString("]\n")
		b.WriteString(content)
		b.WriteString("\n\n")
	}

	if up := strings.TrimSpace(req.GetUserPrompt()); up != "" {
		b.WriteString("[user]\n")
		b.WriteString(up)
	}

	return b.String()
}
