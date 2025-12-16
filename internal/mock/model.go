package mock

type ChatRequest struct {
	Model     string `json:"model"`
	Stream    bool   `json:"stream"`
	MaxTokens int    `json:"max_tokens"`
	Messages  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`

	// Optional overrides (편의)
	Mock *Overrides `json:"mock,omitempty"`
}

type Overrides struct {
	BaseDelayMs     *int     `json:"base_delay_ms,omitempty"`
	JitterMs        *int     `json:"jitter_ms,omitempty"`
	PerTokenDelayMs *int     `json:"per_token_delay_ms,omitempty"`
	ErrorRate       *float64 `json:"error_rate,omitempty"`
	ErrorMode       *string  `json:"error_mode,omitempty"` // "429" | "500" | "mixed"
	ChunkSize       *int     `json:"chunk_size,omitempty"` // chars per chunk
}

type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// StreamChunk SSE chunk (OpenAI-ish)
type StreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content string `json:"content,omitempty"`
			Role    string `json:"role,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}
