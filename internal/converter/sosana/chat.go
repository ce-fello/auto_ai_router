package sosana

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Sosana serves Chat Completions on an OpenAI-compatible endpoint, but only for
// its `-compliant` Gemini aliases. Those models are pinned to Google Gemini and
// are never handed to a third-party provider — which is what makes them usable
// for compliance-bound traffic, and also why Sosana rejects (400) any request
// that would have needed such a provider: tool calling, structured output, an
// output cap, or a text context past the ceiling. The router screens those
// requests off the credential instead, so they route to another provider rather
// than failing.
const (
	// MaxChatContextChars is Sosana's text-context ceiling for compliant chat
	// models. Attachments (image_url parts) do not count towards it.
	MaxChatContextChars = 100_000

	chatCompletionsPath = "/chat/completions"
)

// chatModels are the Sosana chat models this integration routes to.
var chatModels = map[string]struct{}{
	"gemini-flash-compliant": {},
	"gemini-pro-compliant":   {},
}

// unsupportedChatFields are the Chat Completions fields Sosana answers with a
// 400 for compliant models.
var unsupportedChatFields = []string{
	"tools",
	"functions",
	"function_call",
	"response_format",
	"max_tokens",
	"max_completion_tokens",
}

// IsChatCompletionsPath reports whether a request path targets Chat Completions.
func IsChatCompletionsPath(path string) bool {
	return strings.Contains(path, chatCompletionsPath)
}

// SupportedChatModel reports whether modelID is a Sosana chat model.
func SupportedChatModel(modelID string) bool {
	_, ok := chatModels[strings.ToLower(strings.TrimSpace(modelID))]
	return ok
}

// ChatCompletionsURL returns Sosana's OpenAI-compatible Chat Completions endpoint.
func ChatCompletionsURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/") + "/api" + chatCompletionsPath
}

// UnsupportedChatRequest returns a short reason when a Chat Completions request
// cannot be served by Sosana, or "" when it can.
func UnsupportedChatRequest(body []byte, modelID string) string {
	if !SupportedChatModel(modelID) {
		return "model is unsupported"
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return ""
	}
	for _, field := range unsupportedChatFields {
		if hasChatValue(raw[field]) {
			return field + " is unsupported"
		}
	}
	if forcesToolChoice(raw["tool_choice"]) {
		return "tool_choice is unsupported"
	}
	if reason := unsupportedChatChoiceCount(raw["n"]); reason != "" {
		return reason
	}
	if chatContextChars(raw["messages"]) > MaxChatContextChars {
		return "chat context is too large"
	}
	return ""
}

// hasChatValue reports whether a field carries a value that would reach Sosana.
// OpenAI clients routinely send empty containers to mean "unset" (`tools: []`
// from LiteLLM, for example), so those count as absent just like `null` — the
// request is still compliant-routable.
func hasChatValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	switch string(trimmed) {
	case "null", "[]", "{}", `""`:
		return false
	}
	return true
}

// forcesToolChoice reports whether tool_choice asks for a tool call. Without
// tools, "none" and "auto" are no-ops, so they don't disqualify the request.
func forcesToolChoice(raw json.RawMessage) bool {
	if !hasChatValue(raw) {
		return false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		// Object form: {"type": "function", "function": {...}}.
		return true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none", "auto":
		return false
	}
	return true
}

func unsupportedChatChoiceCount(raw json.RawMessage) string {
	if !hasChatValue(raw) {
		return ""
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return "invalid choice count"
	}
	if n == 1 {
		return ""
	}
	return "chat requests support n=1 only"
}

// chatContextChars counts what Sosana counts towards its context ceiling:
// message text, in characters (not bytes), with attachments excluded.
func chatContextChars(raw json.RawMessage) int {
	if !hasChatValue(raw) {
		return 0
	}
	var messages []struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &messages); err != nil {
		return 0
	}
	total := 0
	for _, message := range messages {
		total += chatContentChars(message.Content)
	}
	return total
}

func chatContentChars(raw json.RawMessage) int {
	if !hasChatValue(raw) {
		return 0
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return utf8.RuneCountInString(text)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return 0
	}
	total := 0
	for _, part := range parts {
		if part.Type == "text" {
			total += utf8.RuneCountInString(part.Text)
		}
	}
	return total
}
