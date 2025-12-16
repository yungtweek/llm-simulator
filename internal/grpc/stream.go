package grpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/yungtweek/llm-simulator/internal/config"
	"github.com/yungtweek/llm-simulator/internal/mock"
	"net/http"
	"strconv"
	"time"
)

// ChatCompletionSSEHandler exposes an HTTP handler that streams chat-style SSE responses using the same
// behavior as the gRPC mock. Query params:
// - prompt: required (text to echo/generate from)
// - model: optional model name (default "mock-sse")
// - max_tokens: optional, defaults to cfg.DefaultTokens
// - chunk_size: optional, defaults to cfg.ChunkSize
//
// NOTE: This project currently does not mount an HTTP server; to use SSE in production or demos,
// wire this handler into your own http.Server (TODO: add first-class HTTP entrypoint if needed).
func ChatCompletionSSEHandler(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		model := q.Get("model")
		if model == "" {
			model = "mock-sse"
		}

		prompt := q.Get("prompt")
		if prompt == "" {
			http.Error(w, "prompt is required", http.StatusBadRequest)
			return
		}

		maxTokens := cfg.DefaultTokens
		if v := q.Get("max_tokens"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				maxTokens = n
			}
		}

		chunkSize := cfg.ChunkSize
		if v := q.Get("chunk_size"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				chunkSize = n
			}
		}

		serveChatCompletionSSE(w, r, model, prompt, maxTokens, cfg, chunkSize)
	}
}

func serveChatCompletionSSE(w http.ResponseWriter, r *http.Request, model, prompt string, maxTokens int, cfg config.Config, chunkSize int) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	id := "chatcmpl_mock_" + mock.RandID()
	created := time.Now().Unix()

	chunkSize = defaultInt(chunkSize, defaultInt(cfg.ChunkSize, 12))
	if cfg.Randomize && chunkSize > 1 {
		j := chunkSize / 3
		if j < 1 {
			j = 1
		}
		chunkSize = (chunkSize - j) + mock.RandIntn(j*2+1)
		if chunkSize < 1 {
			chunkSize = 1
		}
	}

	content := mock.BuildOutput(prompt, maxTokens, cfg.EchoPrompt, cfg.StrictTokenMode, cfg.DebugOutputChars, cfg.MaxOutputChars)
	bw := bufio.NewWriter(w)

	// First chunk: role
	first := mock.StreamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
	}
	firstChoice := struct {
		Index int `json:"index"`
		Delta struct {
			Content string `json:"content,omitempty"`
			Role    string `json:"role,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	}{Index: 0}
	firstChoice.Delta.Role = "assistant"
	first.Choices = append(first.Choices, firstChoice)

	if err := writeSSE(bw, first); err != nil {
		return
	}
	if err := bw.Flush(); err != nil {
		return
	}
	flusher.Flush()

	// Content chunks
	for i := 0; i < len(content); i += chunkSize {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		end := i + chunkSize
		if end > len(content) {
			end = len(content)
		}
		part := content[i:end]

		ch := mock.StreamChunk{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
		}
		choice := struct {
			Index int `json:"index"`
			Delta struct {
				Content string `json:"content,omitempty"`
				Role    string `json:"role,omitempty"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		}{Index: 0}
		choice.Delta.Content = part
		ch.Choices = append(ch.Choices, choice)

		if err := writeSSE(bw, ch); err != nil {
			return
		}
		if err := bw.Flush(); err != nil {
			return
		}
		flusher.Flush()

		sleepSSEStreamGap(r.Context(), cfg, part)
	}

	// Done
	doneReason := "stop"
	last := mock.StreamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
	}
	lastChoice := struct {
		Index int `json:"index"`
		Delta struct {
			Content string `json:"content,omitempty"`
			Role    string `json:"role,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	}{Index: 0, FinishReason: &doneReason}
	last.Choices = append(last.Choices, lastChoice)

	if err := writeSSE(bw, last); err != nil {
		return
	}
	if _, err := fmt.Fprint(bw, "data: [DONE]\n\n"); err != nil {
		return
	}
	if err := bw.Flush(); err != nil {
		return
	}
	flusher.Flush()
}

func writeSSE(w *bufio.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return err
	}
	return nil
}

// sleepSSEStreamGap applies the same stream pacing knobs used by the gRPC stream path.
func sleepSSEStreamGap(ctx context.Context, cfg config.Config, delta string) {
	ms := 0

	min := defaultInt(cfg.StreamDelayMinMs, 0)
	max := defaultInt(cfg.StreamDelayMaxMs, 0)
	if max > 0 {
		if max < min {
			max = min
		}
		ms += min
		if max > min {
			ms += mock.RandIntn(max - min + 1)
		}
	}

	if tps := defaultInt(cfg.TokensPerSec, 0); tps > 0 {
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

	per := defaultInt(cfg.PerTokenDelayMs, 0)
	if per > 0 {
		toks := mock.ApproxTokens(delta)
		if toks < 1 {
			toks = 1
		}
		ms += per * toks
	}

	sleepWithContext(ctx, time.Duration(ms)*time.Millisecond)
}
