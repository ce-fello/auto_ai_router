package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mixaill76/auto_ai_router/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCometProvider_OpenAIPrimarySucceeds(t *testing.T) {
	var calls int32

	cometServer := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer comet-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-comet",
			"object":  "chat.completion",
			"model":   "gpt-5.5",
			"choices": []map[string]interface{}{{"message": map[string]string{"role": "assistant", "content": "Hello from comet provider"}}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer cometServer.Close()

	prx := NewTestProxyBuilder().
		WithCredentials(config.CredentialConfig{
			Name:       "cometapi01-openai",
			Type:       config.ProviderTypeOpenAI,
			APIKey:     "comet-key",
			BaseURL:    cometServer.URL + "/v1",
			RPM:        100,
			TPM:        10000,
			IsFallback: false,
		}).
		Build()

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Authorization", "Bearer master-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	prx.ProxyRequest(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Hello from comet provider")
	assert.Equal(t, int32(1), calls)
}

func TestCometProvider_AnthropicPrimaryUsesMessagesAndCacheControl(t *testing.T) {
	var calls int32
	var receivedBody []byte

	cometServer := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "comet-key", r.Header.Get("X-Api-Key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("Anthropic-Version"))
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "msg-comet",
			"type":    "message",
			"role":    "assistant",
			"model":   "claude-sonnet-4.6",
			"content": []map[string]string{{"type": "text", "text": "Hello from comet claude"}},
			"usage": map[string]int{
				"input_tokens":                10,
				"cache_creation_input_tokens": 4,
				"cache_read_input_tokens":     3,
				"output_tokens":               5,
			},
			"stop_reason": "end_turn",
		})
	}))
	defer cometServer.Close()

	prx := NewTestProxyBuilder().
		WithCredentials(config.CredentialConfig{
			Name:       "cometapi01-anthropic",
			Type:       config.ProviderTypeAnthropic,
			APIKey:     "comet-key",
			BaseURL:    cometServer.URL,
			RPM:        100,
			TPM:        10000,
			IsFallback: false,
		}).
		Build()

	reqBody := `{"model":"claude-sonnet-4.6","user":"session-1","messages":[{"role":"system","content":"stable prefix for prompt cache"},{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer master-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	prx.ProxyRequest(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Hello from comet claude")
	assert.Contains(t, string(receivedBody), `"cache_control":{"type":"ephemeral"}`)
	assert.Equal(t, int32(1), calls)
}

func TestCometProvider_RetryableErrorsRetrySameTypeProvider(t *testing.T) {
	retryableStatuses := []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}

	for _, status := range retryableStatuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var cometCalls, backupCalls int32

			cometServer := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&cometCalls, 1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "temporary comet error"})
			}))
			defer cometServer.Close()

			backupServer := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&backupCalls, 1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"id":      "chatcmpl-backup",
					"object":  "chat.completion",
					"model":   "gpt-5.5",
					"choices": []map[string]interface{}{{"message": map[string]string{"role": "assistant", "content": "backup ok"}}},
					"usage":   map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
				})
			}))
			defer backupServer.Close()

			prx := NewTestProxyBuilder().
				WithMaxProviderRetries(1).
				WithCredentials(
					config.CredentialConfig{
						Name:       "cometapi01-openai",
						Type:       config.ProviderTypeOpenAI,
						APIKey:     "comet-key",
						BaseURL:    cometServer.URL + "/v1",
						RPM:        100,
						TPM:        10000,
						IsFallback: false,
					},
					config.CredentialConfig{
						Name:       "openai-backup",
						Type:       config.ProviderTypeOpenAI,
						APIKey:     "backup-key",
						BaseURL:    backupServer.URL + "/v1",
						RPM:        100,
						TPM:        10000,
						IsFallback: false,
					},
				).
				Build()

			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[{"role":"user","content":"Hello"}]}`))
			req.Header.Set("Authorization", "Bearer master-key")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			prx.ProxyRequest(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "backup ok")
			assert.Equal(t, int32(1), cometCalls)
			assert.Equal(t, int32(1), backupCalls)
		})
	}
}

func TestCometProvider_InvalidRequestIsMaskedAndNotRetried(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var cometCalls, backupCalls int32

	cometServer := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&cometCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"comet-internal-secret","code":"invalid_request","group":"overloaded_group_a"}}`))
	}))
	defer cometServer.Close()

	backupServer := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&backupCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer backupServer.Close()

	prx := NewTestProxyBuilder().
		WithLogger(logger).
		WithMaxProviderRetries(1).
		WithCredentials(
			config.CredentialConfig{
				Name:       "cometapi01-openai",
				Type:       config.ProviderTypeOpenAI,
				APIKey:     "comet-key",
				BaseURL:    cometServer.URL + "/v1",
				RPM:        100,
				TPM:        10000,
				IsFallback: false,
			},
			config.CredentialConfig{
				Name:       "openai-backup",
				Type:       config.ProviderTypeOpenAI,
				APIKey:     "backup-key",
				BaseURL:    backupServer.URL + "/v1",
				RPM:        100,
				TPM:        10000,
				IsFallback: false,
			},
		).
		Build()

	reqBody := `{"model":"gpt-5.5","messages":[{"role":"user","content":"super-secret-user-payload"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer master-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	prx.ProxyRequest(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Upstream provider error")
	assert.NotContains(t, w.Body.String(), "comet-internal-secret")
	assert.NotContains(t, w.Body.String(), "overloaded_group_a")
	assert.Equal(t, int32(1), cometCalls)
	assert.Equal(t, int32(0), backupCalls)

	logs := logBuf.String()
	assert.NotContains(t, logs, "comet-internal-secret")
	assert.NotContains(t, logs, "overloaded_group_a")
	assert.Contains(t, logs, "super-secret-user-payload")
	assert.Contains(t, logs, "response_body_masked=true")
}

func TestCometProvider_StrangeErrorWithoutCometTextIsMasked(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cometServer := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"route group shard-17 refused channel alpha","code":"","group":"secret-routing-group"}}`))
	}))
	defer cometServer.Close()

	prx := NewTestProxyBuilder().
		WithLogger(logger).
		WithCredentials(config.CredentialConfig{
			Name:       "cometapi01-openai",
			Type:       config.ProviderTypeOpenAI,
			APIKey:     "comet-key",
			BaseURL:    cometServer.URL + "/v1",
			RPM:        100,
			TPM:        10000,
			IsFallback: false,
		}).
		Build()

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Authorization", "Bearer master-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	prx.ProxyRequest(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Upstream provider error")
	assert.NotContains(t, w.Body.String(), "route group shard-17")
	assert.NotContains(t, w.Body.String(), "secret-routing-group")

	logs := logBuf.String()
	assert.NotContains(t, logs, "route group shard-17")
	assert.NotContains(t, logs, "secret-routing-group")
	assert.Contains(t, logs, "response_body_masked=true")
}

func TestCometProvider_ModelNotFoundIsMaskedAndNotRetried(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var cometCalls, backupCalls int32

	cometServer := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&cometCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"model_not_found","message":"no available channel for group default and model missing-model (distributor)","type":"comet_api_error"}}`))
	}))
	defer cometServer.Close()

	backupServer := newIPv4Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&backupCalls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer backupServer.Close()

	prx := NewTestProxyBuilder().
		WithLogger(logger).
		WithMaxProviderRetries(1).
		WithCredentials(
			config.CredentialConfig{
				Name:       "cometapi01-openai",
				Type:       config.ProviderTypeOpenAI,
				APIKey:     "comet-key",
				BaseURL:    cometServer.URL + "/v1",
				RPM:        100,
				TPM:        10000,
				IsFallback: false,
			},
			config.CredentialConfig{
				Name:       "openai-backup",
				Type:       config.ProviderTypeOpenAI,
				APIKey:     "backup-key",
				BaseURL:    backupServer.URL + "/v1",
				RPM:        100,
				TPM:        10000,
				IsFallback: false,
			},
		).
		Build()

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"missing-model","messages":[{"role":"user","content":"Hello"}]}`))
	req.Header.Set("Authorization", "Bearer master-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	prx.ProxyRequest(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "Upstream provider error")
	assert.NotContains(t, w.Body.String(), "model_not_found")
	assert.NotContains(t, w.Body.String(), "group default")
	assert.Equal(t, int32(1), cometCalls)
	assert.Equal(t, int32(0), backupCalls)

	logs := logBuf.String()
	assert.NotContains(t, logs, "model_not_found")
	assert.NotContains(t, logs, "group default")
	assert.Contains(t, logs, "response_body_masked=true")
}

func TestCometCredentialDetection(t *testing.T) {
	tests := []struct {
		name string
		cred config.CredentialConfig
		want bool
	}{
		{
			name: "production base url",
			cred: config.CredentialConfig{Name: "generic-openai", BaseURL: "https://api.cometapi.com/v1"},
			want: true,
		},
		{
			name: "base url without scheme",
			cred: config.CredentialConfig{Name: "generic-openai", BaseURL: "api.cometapi.com/v1"},
			want: true,
		},
		{
			name: "credential name",
			cred: config.CredentialConfig{Name: "cometapi01-openai", BaseURL: "http://127.0.0.1/v1"},
			want: true,
		},
		{
			name: "unrelated provider",
			cred: config.CredentialConfig{Name: "openai", BaseURL: "https://api.openai.com/v1"},
			want: false,
		},
		{
			name: "lookalike host",
			cred: config.CredentialConfig{Name: "openai", BaseURL: "https://api.cometapi.com.evil.test/v1"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldMaskUpstreamErrors(&tt.cred))
		})
	}
}
