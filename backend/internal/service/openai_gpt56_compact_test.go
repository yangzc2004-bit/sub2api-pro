package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAICodexCompactReasoningEffortForAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-sol","input":"compact me","reasoning":{"effort":"max"}}`)

	tests := []struct {
		name    string
		path    string
		account *Account
		changed bool
		want    string
	}{
		{
			name:    "OpenAI OAuth compact downgrades max",
			path:    "/responses/compact",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			changed: true,
			want:    "xhigh",
		},
		{
			name:    "normal Responses preserves max",
			path:    "/responses",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			want:    "max",
		},
		{
			name:    "API key compact preserves max",
			path:    "/responses/compact",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			want:    "max",
		},
		{
			name:    "non GPT-5.6 compact preserves max",
			path:    "/responses/compact",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			want:    "max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testBody := body
			if tt.name == "non GPT-5.6 compact preserves max" {
				testBody = []byte(`{"model":"gpt-5.5","reasoning":{"effort":"max"}}`)
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)

			got, changed, err := normalizeOpenAICodexCompactReasoningEffortForAccount(c, tt.account, testBody)

			require.NoError(t, err)
			require.Equal(t, tt.changed, changed)
			require.Equal(t, tt.want, gjson.GetBytes(got, "reasoning.effort").String())
		})
	}
}

func TestNormalizeOpenAICodexCompactReasoningEffortSupportsGPT56Family(t *testing.T) {
	for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "openai/gpt-5.6-sol", "gpt-5.6-terra-2026-07-10"} {
		t.Run(model, func(t *testing.T) {
			body := []byte(`{"model":"` + model + `","reasoning":{"effort":"max","summary":"auto"}}`)

			got, changed, err := normalizeOpenAICodexCompactReasoningEffort(body, model)

			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, "xhigh", gjson.GetBytes(got, "reasoning.effort").String())
			require.Equal(t, "auto", gjson.GetBytes(got, "reasoning.summary").String())
		})
	}
}

func TestOpenAIGatewayForwardOAuthCompactDowngradesGPT56Max(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":2}}`)),
		},
	}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{
		ID:          1760,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-account",
		},
		Status:      StatusActive,
		Schedulable: true,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses/compact", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	body := []byte(`{"model":"gpt-5.6-sol","input":"hi","reasoning":{"effort":"max"}}`)
	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, chatgptCodexURL+"/compact", upstream.lastReq.URL.String())
	require.Equal(t, "xhigh", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
}
