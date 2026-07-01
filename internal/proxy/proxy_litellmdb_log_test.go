package proxy

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mixaill76/auto_ai_router/internal/config"
	"github.com/mixaill76/auto_ai_router/internal/converter"
	"github.com/mixaill76/auto_ai_router/internal/litellmdb"
	dbmodels "github.com/mixaill76/auto_ai_router/internal/litellmdb/models"
	routermodels "github.com/mixaill76/auto_ai_router/internal/models"
)

type captureLiteLLMDB struct {
	entry *dbmodels.SpendLogEntry
}

func (m *captureLiteLLMDB) FetchMasterKey(context.Context, string) error { return nil }

func (m *captureLiteLLMDB) ValidateToken(context.Context, string) (*dbmodels.TokenInfo, error) {
	return nil, nil
}

func (m *captureLiteLLMDB) ValidateTokenForModel(context.Context, string, string) (*dbmodels.TokenInfo, error) {
	return nil, nil
}

func (m *captureLiteLLMDB) LogSpend(entry *dbmodels.SpendLogEntry) error {
	m.entry = entry
	return nil
}

func (m *captureLiteLLMDB) FetchModelsForAIR(context.Context, string) ([]config.CredentialConfig, []config.ModelRPMConfig, map[string]*routermodels.ModelPrice, error) {
	return nil, nil, nil, nil
}

func (m *captureLiteLLMDB) IsEnabled() bool { return true }
func (m *captureLiteLLMDB) IsHealthy() bool { return true }
func (m *captureLiteLLMDB) AuthCacheStats() dbmodels.AuthCacheStats {
	return dbmodels.AuthCacheStats{}
}
func (m *captureLiteLLMDB) SpendLoggerStats() dbmodels.SpendLoggerStats {
	return dbmodels.SpendLoggerStats{}
}
func (m *captureLiteLLMDB) ConnectionStats() *pgxpool.Stat { return nil }
func (m *captureLiteLLMDB) GetPool() *pgxpool.Pool         { return nil }
func (m *captureLiteLLMDB) Shutdown(context.Context) error { return nil }

func TestLogSpendToLiteLLMDBUsesTokenTeamOnly(t *testing.T) {
	priceRegistry := routermodels.NewModelPriceRegistry()
	priceRegistry.Update(map[string]*routermodels.ModelPrice{
		"gpt-4o": {
			InputCostPerToken:  0.01,
			OutputCostPerToken: 0.02,
		},
	})

	newLogCtx := func(tokenInfo *litellmdb.TokenInfo) *RequestLogContext {
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		return &RequestLogContext{
			RequestID: "req-1",
			StartTime: time.Date(
				2026, time.June, 27,
				12, 0, 0, 0,
				time.UTC,
			),
			Request: req,
			Token:   "sk-user-token",
			Credential: &config.CredentialConfig{
				Name: "8ee87901-7dc6-4f63-9b58-c9f26a13243b",
				Type: config.ProviderTypeOpenAI,
			},
			ModelID: "gpt-4o",
			TokenUsage: &converter.TokenUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
			},
			Status:     "success",
			HTTPStatus: 200,
			TokenInfo:  tokenInfo,
		}
	}

	t.Run("real team id is preserved", func(t *testing.T) {
		db := &captureLiteLLMDB{}
		p := &Proxy{
			LiteLLMDB:     db,
			priceRegistry: priceRegistry,
			logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		err := p.logSpendToLiteLLMDB(newLogCtx(&litellmdb.TokenInfo{
			Token:          litellmdb.HashToken("sk-user-token"),
			UserID:         "user-1",
			TeamID:         "team-real",
			OrganizationID: "org-1",
		}))

		require.NoError(t, err)
		require.NotNil(t, db.entry)
		assert.Equal(t, "team-real", db.entry.TeamID)
		assert.Equal(t, litellmdb.HashToken("sk-user-token"), db.entry.APIKey)
		assert.InDelta(t, 2.0, db.entry.Spend, 0.000001)
	})

	t.Run("missing team id is not replaced with credential name", func(t *testing.T) {
		db := &captureLiteLLMDB{}
		p := &Proxy{
			LiteLLMDB:     db,
			priceRegistry: priceRegistry,
			logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		err := p.logSpendToLiteLLMDB(newLogCtx(&litellmdb.TokenInfo{
			Token:  litellmdb.HashToken("sk-user-token"),
			UserID: "litellm-master-key",
		}))

		require.NoError(t, err)
		require.NotNil(t, db.entry)
		assert.Empty(t, db.entry.TeamID)
		assert.NotEqual(t, "8ee87901-7dc6-4f63-9b58-c9f26a13243b", db.entry.TeamID)
		assert.Equal(t, litellmdb.HashToken("sk-user-token"), db.entry.APIKey)
		assert.InDelta(t, 2.0, db.entry.Spend, 0.000001)
	})
}
