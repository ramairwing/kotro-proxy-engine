// Package models defines OpenAI-compatible request/response structs used by the
// proxy's streaming cache and middleware pipeline. Struct tags mirror the upstream
// JSON schema to minimize allocation during decode/encode hot paths.
package models

import "encoding/json"

// ChatCompletionRequest is the inbound payload from local IDE agents.
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
}

// ChatMessage represents a single turn in the conversation array.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamChunk is a single SSE data line from OpenAI-compatible providers.
type StreamChunk struct {
	ID      string         `json:"id,omitempty"`
	Object  string         `json:"object,omitempty"`
	Created int64          `json:"created,omitempty"`
	Model   string         `json:"model,omitempty"`
	Choices []StreamChoice `json:"choices"`
}

// StreamChoice holds the delta fragment for one streaming choice.
type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

// StreamDelta is the incremental text payload inside a streaming choice.
type StreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// ExtractPromptState returns the system prompt text and the latest user message
// content — the two inputs used for semantic cache keying.
func (r *ChatCompletionRequest) ExtractPromptState() (systemPrompt, latestUser string) {
	for _, msg := range r.Messages {
		switch msg.Role {
		case "system":
			systemPrompt = msg.Content
		case "user":
			latestUser = msg.Content
		}
	}
	return systemPrompt, latestUser
}

// Clone returns a deep copy suitable for mutation by middleware without aliasing
// the original request body slices.
func (r *ChatCompletionRequest) Clone() *ChatCompletionRequest {
	out := *r
	out.Messages = make([]ChatMessage, len(r.Messages))
	copy(out.Messages, r.Messages)
	return &out
}

// Marshal serializes the request to JSON bytes.
func (r *ChatCompletionRequest) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

// ParseChatCompletionRequest decodes a JSON body into ChatCompletionRequest.
func ParseChatCompletionRequest(body []byte) (*ChatCompletionRequest, error) {
	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	return &req, nil
}
