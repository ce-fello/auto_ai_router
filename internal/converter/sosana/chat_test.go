package sosana

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsURL(t *testing.T) {
	assert.Equal(t, "https://sosana.art/api/chat/completions", ChatCompletionsURL("https://sosana.art"))
	assert.Equal(t, "https://sosana.art/api/chat/completions", ChatCompletionsURL("https://sosana.art/"))
}

func TestIsChatCompletionsPath(t *testing.T) {
	assert.True(t, IsChatCompletionsPath("/v1/chat/completions"))
	assert.True(t, IsChatCompletionsPath("/chat/completions"))
	assert.False(t, IsChatCompletionsPath("/v1/images/generations"))
	assert.False(t, IsChatCompletionsPath("/v1/messages"))
	assert.False(t, IsChatCompletionsPath("/v1/responses"))
}

func TestSupportedChatModel(t *testing.T) {
	assert.True(t, SupportedChatModel("gemini-flash-compliant"))
	assert.True(t, SupportedChatModel("gemini-pro-compliant"))
	assert.True(t, SupportedChatModel(" Gemini-Pro-Compliant "))
	assert.False(t, SupportedChatModel("gemini-pro"))
	assert.False(t, SupportedChatModel("banana-2-1k-compliant"))
	assert.False(t, SupportedChatModel(""))
}

func TestUnsupportedChatRequest(t *testing.T) {
	chatBody := func(extra map[string]any) []byte {
		body := map[string]any{
			"model":    "gemini-pro-compliant",
			"messages": []any{map[string]any{"role": "user", "content": "hello"}},
		}
		for key, value := range extra {
			body[key] = value
		}
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		return raw
	}

	tests := []struct {
		name   string
		extra  map[string]any
		reason string
	}{
		{name: "plain request"},
		{name: "streaming", extra: map[string]any{"stream": true, "stream_options": map[string]any{"include_usage": true}}},
		{name: "sampling params are forwarded", extra: map[string]any{"temperature": 0.2, "top_p": 0.9, "stop": []string{"\n"}}},
		{name: "empty tools mean no tools", extra: map[string]any{"tools": []any{}}},
		{name: "null max_tokens", extra: map[string]any{"max_tokens": nil}},
		{name: "tool_choice none", extra: map[string]any{"tool_choice": "none"}},
		{name: "tool_choice auto", extra: map[string]any{"tool_choice": "auto"}},
		{name: "single choice", extra: map[string]any{"n": 1}},
		{
			name:   "tools",
			extra:  map[string]any{"tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "now"}}}},
			reason: "tools is unsupported",
		},
		{
			name:   "legacy functions",
			extra:  map[string]any{"functions": []any{map[string]any{"name": "now"}}},
			reason: "functions is unsupported",
		},
		{
			name:   "forced tool_choice",
			extra:  map[string]any{"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": "now"}}},
			reason: "tool_choice is unsupported",
		},
		{
			name:   "required tool_choice",
			extra:  map[string]any{"tool_choice": "required"},
			reason: "tool_choice is unsupported",
		},
		{
			name:   "structured output",
			extra:  map[string]any{"response_format": map[string]any{"type": "json_object"}},
			reason: "response_format is unsupported",
		},
		{
			name:   "output cap",
			extra:  map[string]any{"max_tokens": 128},
			reason: "max_tokens is unsupported",
		},
		{
			name:   "new output cap",
			extra:  map[string]any{"max_completion_tokens": 128},
			reason: "max_completion_tokens is unsupported",
		},
		{
			name:   "multiple choices",
			extra:  map[string]any{"n": 2},
			reason: "chat requests support n=1 only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.reason, UnsupportedChatRequest(chatBody(tt.extra), "gemini-pro-compliant"))
		})
	}
}

func TestUnsupportedChatRequestModel(t *testing.T) {
	body := []byte(`{"model":"x","messages":[{"role":"user","content":"hi"}]}`)

	assert.Equal(t, "", UnsupportedChatRequest(body, "gemini-flash-compliant"))
	assert.Equal(t, "model is unsupported", UnsupportedChatRequest(body, "gemini-pro"))
	assert.Equal(t, "model is unsupported", UnsupportedChatRequest(body, "banana-2-1k-compliant"))
}

func TestUnsupportedChatRequestContextLimit(t *testing.T) {
	body := func(content any) []byte {
		raw, err := json.Marshal(map[string]any{
			"model":    "gemini-pro-compliant",
			"messages": []any{map[string]any{"role": "user", "content": content}},
		})
		require.NoError(t, err)
		return raw
	}

	assert.Equal(t, "", UnsupportedChatRequest(body(strings.Repeat("a", MaxChatContextChars)), "gemini-pro-compliant"))
	assert.Equal(t, "chat context is too large",
		UnsupportedChatRequest(body(strings.Repeat("a", MaxChatContextChars+1)), "gemini-pro-compliant"))

	// Multi-byte text is measured in characters, the way Sosana measures it —
	// not in bytes, which would reject a context that still fits.
	assert.Equal(t, "", UnsupportedChatRequest(body(strings.Repeat("я", MaxChatContextChars)), "gemini-pro-compliant"))

	// Attachments do not count towards the ceiling; the text next to them does.
	parts := []any{
		map[string]any{"type": "text", "text": strings.Repeat("a", 10)},
		map[string]any{"type": "image_url", "image_url": map[string]any{
			"url": "data:image/png;base64," + strings.Repeat("A", MaxChatContextChars),
		}},
	}
	assert.Equal(t, "", UnsupportedChatRequest(body(parts), "gemini-pro-compliant"))

	tooMuchText := []any{
		map[string]any{"type": "text", "text": strings.Repeat("a", MaxChatContextChars+1)},
	}
	assert.Equal(t, "chat context is too large", UnsupportedChatRequest(body(tooMuchText), "gemini-pro-compliant"))
}
