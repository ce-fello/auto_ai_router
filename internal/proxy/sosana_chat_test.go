package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mixaill76/auto_ai_router/internal/config"
	"github.com/mixaill76/auto_ai_router/internal/converter/sosana"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sosanaChatCompletion = `{"id":"chatcmpl-1","object":"chat.completion","created":1767225600,` +
	`"model":"gemini-pro-compliant","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},` +
	`"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}}`

func TestProxyRequest_SosanaChatCompletionSuccess(t *testing.T) {
	var seenBody map[string]any
	upstream := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer sosana-key", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&seenBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sosanaChatCompletion))
	}))
	defer upstream.Close()

	prx := newSosanaTestProxy(upstream.URL, nil)
	req := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"gemini-pro-compliant","messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Authorization", "Bearer master-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	prx.ProxyRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gemini-pro-compliant", seenBody["model"])

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	choices, ok := resp["choices"].([]any)
	require.True(t, ok)
	require.Len(t, choices, 1)
	message := choices[0].(map[string]any)["message"].(map[string]any)
	assert.Equal(t, "pong", message["content"])
	assert.NotContains(t, strings.ToLower(w.Body.String()), "sosana")
}

func TestProxyRequest_SosanaChatCompletionUsesProviderModelAlias(t *testing.T) {
	var seenBody map[string]any
	upstream := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/chat/completions", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&seenBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sosanaChatCompletion))
	}))
	defer upstream.Close()

	prx := newSosanaTestProxy(upstream.URL, nil)
	setSosanaTestModels(prx, []config.ModelRPMConfig{
		{Name: "google/gemini-3-pro-compliant", Model: "gemini-pro-compliant", Credential: "sosana"},
	})

	req := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"google/gemini-3-pro-compliant","messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Authorization", "Bearer master-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	prx.ProxyRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	// Sosana is called with its own model name, the client sees its alias back.
	assert.Equal(t, "gemini-pro-compliant", seenBody["model"])

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "google/gemini-3-pro-compliant", resp["model"])
}

func TestProxyRequest_SosanaChatCompletionStreaming(t *testing.T) {
	upstream := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		for _, chunk := range []string{
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1767225600,"model":"gemini-flash-compliant","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}` + "\n\n",
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1767225600,"model":"gemini-flash-compliant","choices":[{"index":0,"delta":{"content":"pong"},"finish_reason":null}]}` + "\n\n",
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1767225600,"model":"gemini-flash-compliant","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n",
			"data: [DONE]\n\n",
		} {
			_, _ = w.Write([]byte(chunk))
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	prx := newSosanaTestProxy(upstream.URL, nil)
	req := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"gemini-flash-compliant","messages":[{"role":"user","content":"ping"}],"stream":true}`))
	req.Header.Set("Authorization", "Bearer master-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	prx.ProxyRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	body := w.Body.String()
	assert.Contains(t, body, `"content":"pong"`)
	assert.Contains(t, body, "data: [DONE]")
}

func TestProxyRequest_SosanaChatRejectsUnsupportedParameters(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "tools",
			body: `{"model":"gemini-pro-compliant","messages":[{"role":"user","content":"ping"}],` +
				`"tools":[{"type":"function","function":{"name":"now"}}]}`,
		},
		{
			name: "response_format",
			body: `{"model":"gemini-pro-compliant","messages":[{"role":"user","content":"ping"}],` +
				`"response_format":{"type":"json_object"}}`,
		},
		{
			name: "max_tokens",
			body: `{"model":"gemini-pro-compliant","messages":[{"role":"user","content":"ping"}],"max_tokens":64}`,
		},
		{
			name: "context over the ceiling",
			body: `{"model":"gemini-pro-compliant","messages":[{"role":"user","content":"` +
				strings.Repeat("a", sosana.MaxChatContextChars+1) + `"}]}`,
		},
		{
			name: "chat model on the image endpoint",
			body: `{"model":"gemini-pro-compliant","prompt":"draw a fox","n":1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			upstream := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))
			defer upstream.Close()

			path := "/v1/chat/completions"
			if tt.name == "chat model on the image endpoint" {
				path = "/v1/images/generations"
			}

			prx := newSosanaTestProxy(upstream.URL, nil)
			req := httptest.NewRequest("POST", path, strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer master-key")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			prx.ProxyRequest(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), unsupportedCredentialRequestMessage)
			assert.NotContains(t, strings.ToLower(w.Body.String()), "sosana")
			assert.False(t, called, "upstream must not be called for an unsupported request")
		})
	}
}

func TestProxyRequest_SosanaChatUpstreamErrorMasked(t *testing.T) {
	upstream := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"` + "`gemini-pro-compliant` is served exclusively by the internal Gemini provider" + `"}`))
	}))
	defer upstream.Close()

	prx := newSosanaTestProxy(upstream.URL, nil)
	req := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"gemini-pro-compliant","messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Authorization", "Bearer master-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	prx.ProxyRequest(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.NotContains(t, strings.ToLower(w.Body.String()), "gemini provider")
	assert.NotContains(t, strings.ToLower(w.Body.String()), "sosana")
}
