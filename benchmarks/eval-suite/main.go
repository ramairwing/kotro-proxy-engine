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
	deepseekKey := os.Getenv("DEEPSEEK_API_KEY")
	dashscopeKey := os.Getenv("DASHSCOPE_API_KEY")
	
	if deepseekKey == "" && dashscopeKey == "" {
		fmt.Println("Error: DEEPSEEK_API_KEY or DASHSCOPE_API_KEY is required in environment.")
		os.Exit(1)
	}

	fmt.Println("Coordinated Cache Evaluation (DeepSeek & Qwen)")
	fmt.Println("==================================================")

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

	if deepseekKey != "" {
		fmt.Println("Running Scenario A: DeepSeek Full Digest Strategy")
		runScenario(deepseekKey, "Scenario A: DeepSeek Full Digest", cache.StrategyFullDigest, "https://api.deepseek.com", "deepseek-chat", system, codeDump, out)

		fmt.Println("\nRunning Scenario B: DeepSeek Window N Strategy")
		runScenario(deepseekKey, "Scenario B: DeepSeek Window N", cache.StrategyWindowN, "https://api.deepseek.com", "deepseek-chat", system, codeDump, out)
	}

	if dashscopeKey != "" {
		fmt.Println("\nRunning Scenario C: Qwen Full Digest Strategy")
		runScenario(dashscopeKey, "Scenario C: Qwen Full Digest", cache.StrategyFullDigest, "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", "qwen-plus", system, codeDump, out)

		fmt.Println("\nRunning Scenario D: Qwen Window N Strategy")
		runScenario(dashscopeKey, "Scenario D: Qwen Window N", cache.StrategyWindowN, "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", "qwen-plus", system, codeDump, out)
	}
}

func mustFlexText(text string) models.FlexContent {
	b, _ := json.Marshal(text)
	var fc models.FlexContent
	fc.UnmarshalJSON(b)
	return fc
}

func runScenario(apiKey, name string, strategy cache.CacheKeyStrategy, upstreamURL, model string, system models.ChatMessage, codeDump string, out io.Writer) {
	// 1. Setup local proxy
	storePath := os.TempDir() + "/korto-eval-" + fmt.Sprintf("%d", time.Now().UnixNano()) + ".db"
	store, _ := cache.Open(storePath)
	defer os.Remove(storePath)
	
	registry := metrics.NewRegistry()

	opts := proxy.Options{
		UpstreamURL:       upstreamURL,
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
			Model:  model,
			Stream: true, // Must stream for proxy to intercept!
			StreamOpts: &models.StreamOptions{
				IncludeUsage: true,
			},
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

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("API Error (HTTP %d):\n", resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Body: %s\n", string(body))
			resp.Body.Close()
			return
		}

		if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
			fmt.Printf("Unexpected Content-Type: %s\n", resp.Header.Get("Content-Type"))
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Body: %s\n", string(body))
			resp.Body.Close()
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
		out.Write([]byte(fmt.Sprintf("- **Server Prompt Tokens**: %d\n", usage.PromptTokens)))
		out.Write([]byte(fmt.Sprintf("- **Server Cache Hits**: %d\n", usage.PromptCacheHitTokens)))
		out.Write([]byte(fmt.Sprintf("- **Server Cache Misses**: %d\n\n", usage.PromptCacheMissTokens)))
	}
}
