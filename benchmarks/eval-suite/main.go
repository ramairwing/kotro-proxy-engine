package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/kortolabs/proxy-engine/internal/cache"
	"github.com/kortolabs/proxy-engine/internal/metrics"
	"github.com/kortolabs/proxy-engine/internal/models"
	"github.com/kortolabs/proxy-engine/internal/proxy"
)

func main() {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: DEEPSEEK_API_KEY is required in environment.")
		os.Exit(1)
	}

	fmt.Println("DeepSeek V4 + Proxy Coordinated Cache Evaluation")
	fmt.Println("================================================")

	out, err := os.OpenFile("benchmarks/eval-suite/RESULTS.md", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Printf("Failed to create RESULTS.md: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	out.WriteString("# Proxy Benchmark Results\n\n")

	system := models.ChatMessage{
		Role:    "system",
		Content: mustFlexText("You are an expert systems engineer. Adhere strictly to the requested architecture."),
	}
	codeDump := fmt.Sprintf("<file name=\"core.go\">\npackage core\n\n%s\n</file>", strings.Repeat("// complex logic simulation loop\nfunc mock() {}\n", 200))

	fmt.Println("Running Scenario A: Full Digest Strategy")
	runScenario(apiKey, "Scenario A: Full Digest", cache.StrategyFullDigest, system, codeDump, out)

	fmt.Println("\nRunning Scenario B: Window N Strategy")
	runScenario(apiKey, "Scenario B: Window N", cache.StrategyWindowN, system, codeDump, out)
}

func mustFlexText(text string) models.FlexContent {
	b, _ := json.Marshal(text)
	var fc models.FlexContent
	fc.UnmarshalJSON(b)
	return fc
}

func runScenario(apiKey, name string, strategy cache.CacheKeyStrategy, system models.ChatMessage, codeDump string, out io.Writer) {
	// 1. Setup local proxy
	storePath := os.TempDir() + "/korto-eval-" + fmt.Sprintf("%d", time.Now().UnixNano()) + ".db"
	store, _ := cache.Open(storePath)
	defer os.Remove(storePath)
	
	registry := metrics.NewRegistry()

	opts := proxy.Options{
		UpstreamURL:       "https://api.deepseek.com",
		EnableCache:       true,
		EnableRedaction:   false,
		EnableCompression: false, // Testing just semantic caching
		CacheHitDelay:     0,
		CacheKeyStrategy:  strategy,
		CacheWindowSize:   4,
		Metrics:           registry,
	}
	h, err := proxy.NewHandler(opts, store, slog.Default())
	if err != nil {
		fmt.Printf("failed to init proxy: %v\n", err)
		return
	}
	ts := httptest.NewServer(h)
	defer ts.Close()

	out.Write([]byte(fmt.Sprintf("## %s\n\n", name)))

	// Turns
	var history []models.ChatMessage
	queries := []string{
		"What is the time complexity of the core loop?",
		"Can you optimize it to O(1)?",
		"Write the hash map implementation.",
	}
	assistantReplies := []string{
		"The time complexity is O(N).",
		"Yes, by using a hash map.",
		"Here is the map.",
	}

	for turn := 1; turn <= 3; turn++ {
		query := queries[turn-1]
		
		req := &models.ChatCompletionRequest{
			Model:  "deepseek-chat",
			Stream: true, // Must stream for proxy to intercept!
		}
		
		// Build payload like a sloppy IDE
		req.Messages = append(req.Messages, models.ChatMessage{Role: "user", Content: mustFlexText(query)})
		req.Messages = append(req.Messages, models.ChatMessage{Role: "user", Content: mustFlexText(codeDump)})
		req.Messages = append(req.Messages, history...)
		req.Messages = append(req.Messages, system)

		// Get initial cache hits
		hitsBefore := registry.Snapshot().CacheHits5m

		// Execute
		bodyBytes, _ := json.Marshal(req)
		httpReq, _ := http.NewRequest("POST", ts.URL+"/chat/completions", bytes.NewBuffer(bodyBytes))
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			fmt.Printf("Request failed: %v\n", err)
			return
		}

		var usage models.DeepSeekUsage
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				data := line[6:]
				if data == "[DONE]" {
					continue
				}
				var chunk models.StreamChunk
				if err := json.Unmarshal([]byte(data), &chunk); err == nil {
					if chunk.Usage != nil {
						usage = *chunk.Usage
					}
				}
			}
		}
		resp.Body.Close()

		hitsAfter := registry.Snapshot().CacheHits5m
		localHit := (hitsAfter > hitsBefore)

		// Record History for next turn
		history = append(history, models.ChatMessage{Role: "user", Content: mustFlexText(query)})
		history = append(history, models.ChatMessage{Role: "assistant", Content: mustFlexText(assistantReplies[turn-1])})

		fmt.Printf("Turn %d | Proxy Hit: %v | Server Hit: %d | Server Miss: %d\n", turn, localHit, usage.PromptCacheHitTokens, usage.PromptCacheMissTokens)

		out.Write([]byte(fmt.Sprintf("### Turn %d\n", turn)))
		if localHit {
			out.Write([]byte("- **Local Proxy Status**: 🟢 HIT (100% Upstream Tokens Saved!)\n"))
		} else {
			out.Write([]byte("- **Local Proxy Status**: 🔴 MISS\n"))
		}
		out.Write([]byte(fmt.Sprintf("- **DeepSeek Prompt Tokens**: %d\n", usage.PromptTokens)))
		out.Write([]byte(fmt.Sprintf("- **DeepSeek Cache Hits**: %d\n", usage.PromptCacheHitTokens)))
		out.Write([]byte(fmt.Sprintf("- **DeepSeek Cache Misses**: %d\n\n", usage.PromptCacheMissTokens)))
	}
}
