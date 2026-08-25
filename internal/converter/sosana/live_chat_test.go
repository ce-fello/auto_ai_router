package sosana

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSosanaChatLiveAcceptance(t *testing.T) {
	if os.Getenv("SOSANA_ACCEPTANCE") != "1" {
		t.Skip("SOSANA_ACCEPTANCE=1 not set, skipping paid Sosana live acceptance test")
	}

	apiKey := os.Getenv("SOSANA_API_KEY")
	require.NotEmpty(t, apiKey, "SOSANA_API_KEY is required for Sosana live acceptance test")

	baseURL := os.Getenv("SOSANA_BASE_URL")
	if baseURL == "" {
		baseURL = "https://sosana.art"
	}
	model := os.Getenv("SOSANA_CHAT_MODEL")
	if model == "" {
		model = "gemini-flash-compliant"
	}
	require.True(t, SupportedChatModel(model), "SOSANA_CHAT_MODEL must be a Sosana chat model")

	body, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": []any{map[string]any{"role": "user", "content": `Say "pong" and nothing else.`}},
	})
	require.NoError(t, err)
	require.Empty(t, UnsupportedChatRequest(body, model))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ChatCompletionsURL(baseURL), bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxResultErrorBytes))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "response body: %s", string(rawBody))

	var completion struct {
		Object  string `json:"object"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(rawBody, &completion))
	assert.Equal(t, "chat.completion", completion.Object)
	require.Len(t, completion.Choices, 1)
	assert.NotEmpty(t, completion.Choices[0].Message.Content)
	assert.Positive(t, completion.Usage.PromptTokens)
}
