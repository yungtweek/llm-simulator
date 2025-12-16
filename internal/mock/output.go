package mock

import "fmt"

// BuildOutput generates a mock completion string using the same sizing rules as the gRPC simulator.
// - If strictTokenMode is true, length is based on maxTokens (~4 chars per token).
// - debugChars can force a fixed size when non-zero.
// - maxChars caps the output length when positive.
func BuildOutput(prompt string, maxTokens int, echoPrompt bool, strictTokenMode bool, debugChars int, maxChars int) string {
	target := debugChars
	if target == 0 {
		target = 512
	}
	if strictTokenMode {
		if maxTokens <= 0 {
			maxTokens = 128
		}
		target = maxTokens * 4
	}
	if target < 64 {
		target = 64
	}
	cap := maxChars
	if cap == 0 {
		cap = 4096
	}
	if cap > 0 && target > cap {
		target = cap
	}

	prefix := ""
	if echoPrompt {
		p := trim(prompt, 140)
		prefix = fmt.Sprintf("Mock answer for: %q\n\n", p)
	}
	s := prefix +
		"This is a server-streaming mock response for benchmarking Kafka/worker throughput. \n" +
		"It simulates latency, errors, and chunked deltas. \n"

	for len(s) < target {
		s += "[mock-token] "
	}
	return s[:target]
}

// ApproxTokens provides a rough token estimate (4 runes ~= 1 token).
func ApproxTokens(s string) int {
	if s == "" {
		return 0
	}
	r := len([]rune(s))
	return (r + 3) / 4
}
