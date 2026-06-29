// Command mockupstream is an isolated test harness that mimics OpenAI's
// /v1/chat/completions SSE endpoint with intentional chunked delivery and
// sleep intervals to validate streaming stability offline.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

const listenAddr = ":9000"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleChatCompletions)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("mock upstream listening on %s", listenAddr)
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

	// Echo a short response derived from the latest user message
	userMsg := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			userMsg = m.Content
		}
	}
	reply := fmt.Sprintf("Mock upstream received: %s", truncate(userMsg, 80))
	tokens := strings.Fields(reply)
	if len(tokens) == 0 {
		tokens = []string{"empty"}
	}

	for i, tok := range tokens {
		chunk := map[string]any{
			"id":      "mock-chunk",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   req.Model,
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
		time.Sleep(20 * time.Millisecond)
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
