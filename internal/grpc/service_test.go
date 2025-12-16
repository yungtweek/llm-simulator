package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yungtweek/llm-simulator/internal/config"
	"github.com/yungtweek/llm-simulator/internal/mock"

	llmv1 "github.com/yungtweek/llm-simulator/gen"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TestChatCompletionSuccess verifies the unary RPC returns deterministic output, finish reason, and token accounting
// when all delays and error injection are disabled.
func TestChatCompletionSuccess(t *testing.T) {
	cfg := config.Config{
		BaseDelayMs:      0,
		JitterMs:         0,
		PerTokenDelayMs:  0,
		ErrorRate:        0,
		ErrorMode:        "mixed",
		DefaultTokens:    0,
		ChunkSize:        16,
		StreamDelayMinMs: 0,
		StreamDelayMaxMs: 0,
	}

	svc := NewMockLlmService(cfg)

	req := &llmv1.ChatCompletionRequest{
		Model:        "gpt-mock",
		SystemPrompt: "you are helpful",
		UserPrompt:   "tell me a joke",
		Context: []*llmv1.ChatMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
		},
		MaxTokens: 12,
	}

	resp, err := svc.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatCompletion unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatalf("ChatCompletion returned nil response without error")
	}

	prompt := buildPromptForTokens(req)
	expected := mock.BuildOutput(
		prompt,
		int(req.GetMaxTokens()),
		cfg.EchoPrompt,
		cfg.StrictTokenMode,
		cfg.DebugOutputChars,
		cfg.MaxOutputChars,
	)

	if resp.OutputText != expected {
		t.Fatalf("output mismatch")
	}
	if resp.FinishReason != "stop" {
		t.Fatalf("finish reason mismatch: %q", resp.FinishReason)
	}

	pt := int32(mock.ApproxTokens(prompt))
	ct := int32(mock.ApproxTokens(expected))
	if resp.PromptTokens != pt || resp.CompletionTokens != ct || resp.TotalTokens != pt+ct {
		t.Fatalf("token accounting mismatch: %+v expected prompt=%d completion=%d", resp, pt, ct)
	}
	if resp.LatencyMs < 0 {
		t.Fatalf("latency should be non-negative")
	}
}

// TestChatCompletionErrors verifies error injection maps to the expected gRPC status codes for different modes
// (ResourceExhausted for 429, Internal for 500, and either for mixed).
func TestChatCompletionErrors(t *testing.T) {
	tests := []struct {
		mode   string
		expect codes.Code
	}{
		{mode: "429", expect: codes.ResourceExhausted},
		{mode: "500", expect: codes.Internal},
		{mode: "mixed", expect: codes.OK}, // allow either ResourceExhausted or Internal
	}

	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			cfg := config.Config{
				ErrorRate: 1, // always fail
				ErrorMode: tc.mode,
			}
			svc := NewMockLlmService(cfg)
			_, err := svc.ChatCompletion(context.Background(), &llmv1.ChatCompletionRequest{MaxTokens: 1})
			if err == nil {
				t.Fatalf("expected error")
			}
			got := status.Code(err)
			if tc.expect == codes.OK {
				if got != codes.ResourceExhausted && got != codes.Internal {
					t.Fatalf("unexpected code: %v", got)
				}
				return
			}
			if got != tc.expect {
				t.Fatalf("expected %v, got %v", tc.expect, got)
			}
		})
	}
}

// TestChatCompletionStream verifies server-streaming behavior: chunking respects ChunkSize, intermediate chunks
// have empty finish reason, reassembled deltas match the deterministic output, and the final chunk carries
// finish reason and token/latency fields.
func TestChatCompletionStream(t *testing.T) {
	cfg := config.Config{
		BaseDelayMs:      0,
		JitterMs:         0,
		PerTokenDelayMs:  0,
		ErrorRate:        0,
		ErrorMode:        "mixed",
		DefaultTokens:    0,
		ChunkSize:        7,
		StreamDelayMinMs: 0,
		StreamDelayMaxMs: 0,
	}

	svc := NewMockLlmService(cfg)

	req := &llmv1.ChatCompletionRequest{
		Model:        "mock-stream",
		SystemPrompt: "sys",
		UserPrompt:   "stream this content",
		MaxTokens:    10,
	}

	fs := &fakeStream{ctx: context.Background()}
	if err := svc.ChatCompletionStream(req, fs); err != nil {
		t.Fatalf("ChatCompletionStream err: %v", err)
	}

	prompt := buildPromptForTokens(req)
	out := mock.BuildOutput(
		prompt,
		int(req.GetMaxTokens()),
		cfg.EchoPrompt,
		cfg.StrictTokenMode,
		cfg.DebugOutputChars,
		cfg.MaxOutputChars,
	)
	expectedChunks := (len(out) + cfg.ChunkSize - 1) / cfg.ChunkSize

	if len(fs.sent) != expectedChunks+1 { // +1 final chunk
		t.Fatalf("expected %d chunks, got %d", expectedChunks+1, len(fs.sent))
	}

	var assembled strings.Builder
	for i := 0; i < expectedChunks; i++ {
		part := fs.sent[i].GetText()
		if len(part) == 0 || len(part) > cfg.ChunkSize {
			t.Fatalf("chunk %d size invalid: %d", i, len(part))
		}
		assembled.WriteString(part)
		if fs.sent[i].FinishReason != "" {
			t.Fatalf("finish reason should be empty on intermediate chunks")
		}
	}
	if assembled.String() != out {
		t.Fatalf("reassembled stream mismatch")
	}

	last := fs.sent[len(fs.sent)-1]
	if last.FinishReason != "stop" {
		t.Fatalf("unexpected finish reason: %q", last.FinishReason)
	}
	pt := int32(mock.ApproxTokens(prompt))
	ct := int32(mock.ApproxTokens(out))
	if last.PromptTokens != pt || last.CompletionTokens != ct || last.TotalTokens != pt+ct {
		t.Fatalf("final token counts mismatch: %+v", last)
	}
	if last.LatencyMs < 0 {
		t.Fatalf("latency should be non-negative")
	}
}

// TestChatCompletionStreamError verifies that when error injection triggers before streaming starts, the RPC
// returns the expected status code and sends no chunks.
func TestChatCompletionStreamError(t *testing.T) {
	cfg := config.Config{
		ErrorRate: 1,
		ErrorMode: "500",
	}

	svc := NewMockLlmService(cfg)
	fs := &fakeStream{ctx: context.Background()}
	err := svc.ChatCompletionStream(&llmv1.ChatCompletionRequest{}, fs)
	if err == nil {
		t.Fatalf("expected error")
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected Internal, got %v", status.Code(err))
	}
	if len(fs.sent) != 1 {
		t.Fatalf("expected one failed chunk on error, got %d", len(fs.sent))
	}
	if fs.sent[0].Type != "failed" || fs.sent[0].FinishReason == "" {
		t.Fatalf("expected failed chunk with finish reason, got %+v", fs.sent[0])
	}
}

// fakeStream satisfies llmv1.LlmService_ChatCompletionStreamServer for testing.
type fakeStream struct {
	ctx     context.Context
	sent    []*llmv1.ChatCompletionChunkResponse
	header  metadata.MD
	trailer metadata.MD
	onSend  func(res *llmv1.ChatCompletionChunkResponse)
}

func (f *fakeStream) Send(res *llmv1.ChatCompletionChunkResponse) error {
	f.sent = append(f.sent, res)
	if f.onSend != nil {
		f.onSend(res)
	}
	return nil
}

func (f *fakeStream) SetHeader(md metadata.MD) error {
	f.header = md
	return nil
}

func (f *fakeStream) SendHeader(md metadata.MD) error {
	f.header = md
	return nil
}

func (f *fakeStream) SetTrailer(md metadata.MD) {
	f.trailer = md
}

func (f *fakeStream) Context() context.Context {
	return f.ctx
}

func (f *fakeStream) SendMsg(m interface{}) error {
	_, ok := m.(*llmv1.ChatCompletionChunkResponse)
	if !ok {
		return fmt.Errorf("unexpected message type %T", m)
	}
	return nil
}

func (f *fakeStream) RecvMsg(interface{}) error {
	return nil
}

// TestChatCompletionStreamContextCanceled verifies the streaming RPC stops promptly when the client context
// is canceled mid-stream, returning a canceled error and not sending the final finish chunk.
func TestChatCompletionStreamContextCanceled(t *testing.T) {
	cfg := config.Config{
		BaseDelayMs:      0,
		JitterMs:         0,
		PerTokenDelayMs:  0,
		ErrorRate:        0,
		ErrorMode:        "mixed",
		DefaultTokens:    0,
		ChunkSize:        4,
		StreamDelayMinMs: 0,
		StreamDelayMaxMs: 0,
	}

	svc := NewMockLlmService(cfg)

	req := &llmv1.ChatCompletionRequest{
		Model:      "mock-stream",
		UserPrompt: "cancel me",
		MaxTokens:  64,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fs := &fakeStream{ctx: ctx}
	fs.onSend = func(res *llmv1.ChatCompletionChunkResponse) {
		// Cancel after receiving the first non-empty delta chunk.
		if res.GetText() != "" {
			cancel()
		}
	}

	err := svc.ChatCompletionStream(req, fs)
	if err == nil {
		t.Fatalf("expected cancellation error")
	}

	// Depending on where cancellation is observed, it may surface as context.Canceled (possibly wrapped) or a gRPC Canceled status.
	if !errors.Is(err, context.Canceled) && status.Code(err) != codes.Canceled {
		t.Fatalf("expected canceled, got %v", err)
	}

	if len(fs.sent) == 0 {
		t.Fatalf("expected at least one chunk before cancellation")
	}

	// Ensure we did not send the final finish chunk.
	last := fs.sent[len(fs.sent)-1]
	if last.GetFinishReason() == "stop" {
		t.Fatalf("should not send final finish chunk when canceled")
	}
}
