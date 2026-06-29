// Command mockupstream is an isolated test harness that mimics OpenAI and Anthropic
// streaming endpoints with intentional chunked delivery for offline validation.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const listenAddr = ":9000"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleChatCompletions)
	mux.HandleFunc("/v1/messages", handleAnthropicMessages)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("mock upstream listening on %s (OpenAI + Anthropic)", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	if !req.Stream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"mock","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"mock non-stream"}}]}`))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	userMsg := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			userMsg = m.Content
		}
	}
	streamOpenAI(w, flusher, req.Model, userMsg)
}

func handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Model    string `json:"model"`
		System   string `json:"system"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	if !req.Stream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"mock","type":"message","role":"assistant","content":[{"type":"text","text":"mock non-stream"}]}`))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	userMsg := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			userMsg = m.Content
		}
	}
	streamAnthropic(w, flusher, req.Model, userMsg)
}

func streamOpenAI(w http.ResponseWriter, flusher http.Flusher, model, userMsg string) {
	reply := fmt.Sprintf("Mock upstream received: %s", truncate(userMsg, 80))
	tokens := expandTokens(strings.Fields(reply))
	delay := chunkDelay()

	for i, tok := range tokens {
		chunk := map[string]any{
			"id":      "mock-chunk",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]string{
					"content": tok + " ",
				},
				"finish_reason": nil,
			}},
		}
		if i == len(tokens)-1 {
			chunk["choices"].([]map[string]any)[0]["finish_reason"] = "stop"
		}

		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		time.Sleep(delay)
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func streamAnthropic(w http.ResponseWriter, flusher http.Flusher, model, userMsg string) {
	writeSSE(w, flusher, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":    "mock-msg",
			"type":  "message",
			"role":  "assistant",
			"model": model,
		},
	})
	writeSSE(w, flusher, "content_block_start", map[string]any{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]any{
			"type": "text",
			"text": "",
		},
	})

	reply := fmt.Sprintf("Anthropic mock received: %s", truncate(userMsg, 80))
	tokens := expandTokens(strings.Fields(reply))
	delay := chunkDelay()

	for _, tok := range tokens {
		writeSSE(w, flusher, "content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": tok + " ",
			},
		})
		time.Sleep(delay)
	}

	writeSSE(w, flusher, "content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": 0,
	})
	writeSSE(w, flusher, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
	})
	writeSSE(w, flusher, "message_stop", map[string]any{"type": "message_stop"})
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, payload map[string]any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func chunkDelay() time.Duration {
	if v := os.Getenv("MOCK_CHUNK_DELAY_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 20 * time.Millisecond
}

func expandTokens(tokens []string) []string {
	if len(tokens) == 0 {
		tokens = []string{"empty"}
	}
	min := 8
	if v := os.Getenv("MOCK_MIN_CHUNKS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > min {
			min = n
		}
	}
	out := make([]string, 0, min)
	for len(out) < min {
		out = append(out, tokens...)
	}
	return out[:min]
}
