package mock

import (
	"math/rand"
	"sync"
	"time"
)

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

var rngMu sync.Mutex

func RandIntn(n int) int {
	if n <= 0 {
		return 0
	}
	rngMu.Lock()
	defer rngMu.Unlock()
	return rng.Intn(n)
}

func RandFloat64() float64 {
	rngMu.Lock()
	defer rngMu.Unlock()
	return rng.Float64()
}

func pickErrorStatus(mode string) int {
	switch mode {
	case "429":
		return 429
	case "500":
		return 500
	default:
		// mixed
		if RandIntn(2) == 0 {
			return 429
		}
		return 500
	}
}

func lastUserMessage(req ChatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content
		}
	}
	return ""
}

func trim(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}
func RandID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 10)
	for i := range b {
		b[i] = letters[RandIntn(len(letters))]
	}
	return string(b)
}
