package grpc

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yungtweek/llm-simulator/internal/config"
	"github.com/yungtweek/llm-simulator/internal/mock"
)

func TestStreamSSEAlignsWithGrpcOutput(t *testing.T) {
	cfg := config.Config{
		ChunkSize:       7,
		StrictTokenMode: true,
		MaxOutputChars:  256,
		// Keep randomization off so chunking is deterministic.
	}

	prompt := "sse prompt"
	maxTokens := 10
	expected := mock.BuildOutput(prompt, maxTokens, cfg.EchoPrompt, cfg.StrictTokenMode, cfg.DebugOutputChars, cfg.MaxOutputChars)
	expectedChunks := (len(expected) + cfg.ChunkSize - 1) / cfg.ChunkSize

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	serveChatCompletionSSE(rr, req, "mock-model", prompt, maxTokens, cfg, cfg.ChunkSize)

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(rr.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content type not set for SSE: %q", rr.Header().Get("Content-Type"))
	}

	result := parseSSE(t, body)
	chunks := result.chunks

	// First chunk should carry role only.
	first := chunks[0]
	if len(first.Choices) != 1 || first.Choices[0].Delta.Role != "assistant" {
		t.Fatalf("first chunk missing assistant role: %+v", first)
	}

	// Last chunk should carry finish_reason stop.
	last := chunks[len(chunks)-1]
	if len(last.Choices) != 1 || last.Choices[0].FinishReason == nil || *last.Choices[0].FinishReason != "stop" {
		t.Fatalf("final chunk missing finish_reason stop: %+v", last)
	}

	var assembled strings.Builder
	for i := 1; i < len(chunks)-1; i++ {
		delta := chunks[i].Choices[0].Delta.Content
		if delta == "" {
			t.Fatalf("chunk %d has empty content", i)
		}
		if len(delta) > cfg.ChunkSize {
			t.Fatalf("chunk %d exceeds chunk size: %d > %d", i, len(delta), cfg.ChunkSize)
		}
		assembled.WriteString(delta)
	}

	if got := assembled.String(); got != expected {
		t.Fatalf("reassembled content mismatch\nexpected len=%d\ngot len=%d", len(expected), len(got))
	}

	if gotChunks := len(chunks) - 2; gotChunks != expectedChunks {
		t.Fatalf("delta chunk count mismatch: got %d, expected %d", gotChunks, expectedChunks)
	}
}

func TestNewSSEHandlerUsesQueryParams(t *testing.T) {
	cfg := config.Config{
		ChunkSize:       6,
		DefaultTokens:   5,
		StrictTokenMode: true,
		MaxOutputChars:  200,
	}

	handler := ChatCompletionSSEHandler(cfg)

	req := httptest.NewRequest("GET", "/?prompt=handler%20prompt&max_tokens=6&model=handler&chunk_size=4", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("handler returned non-200: %d body=%s", rr.Code, rr.Body.String())
	}
	body := strings.TrimSpace(rr.Body.String())

	result := parseSSE(t, body)
	chunks := result.chunks

	if len(chunks) < 2 {
		t.Fatalf("expected role chunk and final chunk, got %d", len(chunks))
	}

	prompt := "handler prompt"
	expected := mock.BuildOutput(prompt, 6, cfg.EchoPrompt, cfg.StrictTokenMode, cfg.DebugOutputChars, cfg.MaxOutputChars)
	expectedChunks := (len(expected) + 4 - 1) / 4 // chunk_size override=4

	var assembled strings.Builder
	for i := 1; i < len(chunks)-1; i++ {
		delta := chunks[i].Choices[0].Delta.Content
		if len(delta) > 4 {
			t.Fatalf("chunk size exceeded: %d", len(delta))
		}
		assembled.WriteString(delta)
	}

	if got := assembled.String(); got != expected {
		t.Fatalf("assembled content mismatch: len got=%d expected=%d", len(got), len(expected))
	}
	if gotChunks := len(chunks) - 2; gotChunks != expectedChunks {
		t.Fatalf("delta chunk count mismatch: got %d, expected %d", gotChunks, expectedChunks)
	}
}

// parseSSE extracts chunks and verifies presence of [DONE].
func parseSSE(t *testing.T, body string) (result struct {
	chunks []mock.StreamChunk
	done   bool
}) {
	t.Helper()

	rawEvents := strings.Split(body, "\n\n")
	if len(rawEvents) < 3 { // role chunk + at least one delta + done + [DONE]
		t.Fatalf("unexpected number of SSE events: %d\nbody:\n%s", len(rawEvents), body)
	}

	for _, evt := range rawEvents {
		evt = strings.TrimSpace(evt)
		if !strings.HasPrefix(evt, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(evt, "data: ")
		if payload == "[DONE]" {
			result.done = true
			continue
		}

		var ch mock.StreamChunk
		if err := json.Unmarshal([]byte(payload), &ch); err != nil {
			t.Fatalf("failed to unmarshal SSE chunk: %v\npayload: %s", err, payload)
		}
		result.chunks = append(result.chunks, ch)
	}

	if !result.done {
		t.Fatalf("missing [DONE] marker")
	}
	return result
}
