//go:build unit

package service

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestAccountTestService_QwenWebFlow(t *testing.T) {
	ctx, recorder := newTestContext()
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusOK, "umx.wu('mid-token')"),
			newJSONResponse(http.StatusOK, `{"success":true,"data":{"id":"chat-1"}}`),
			newJSONResponse(http.StatusOK, strings.Join([]string{
				`data: {"choices":[{"delta":{"role":"assistant","content":"Hello","phase":"answer"}}]}`,
				``,
				`data: {"choices":[{"delta":{"content":" Qwen","phase":"answer"},"finish_reason":"stop"}]}`,
				``,
			}, "\n")),
		},
	}
	repo := &openAIAccountTestRepo{}
	account := &Account{
		ID:          7,
		Platform:    PlatformQwen,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"auth_token": "token", "cookie": "a=b"},
		Concurrency: 1,
	}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{},
	}

	err := svc.testQwenAccountConnection(ctx, account, "qwen3.6-plus", "hi")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "https://sg-wum.alibaba.com/w/wu.json", upstream.requests[0].URL.String())
	require.Equal(t, "/api/v2/chats/new", upstream.requests[1].URL.Path)
	require.Equal(t, "/api/v2/chat/completions", upstream.requests[2].URL.Path)
	require.Equal(t, "chat-1", upstream.requests[2].URL.Query().Get("chat_id"))
	require.Contains(t, recorder.Body.String(), `"type":"content","text":"Hello"`)
	require.Contains(t, recorder.Body.String(), `"type":"content","text":" Qwen"`)
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_QwenWebFlowEmptyContentFails(t *testing.T) {
	ctx, recorder := newTestContext()
	upstream := &queuedHTTPUpstream{
		responses: []*http.Response{
			newJSONResponse(http.StatusOK, "umx.wu('mid-token')"),
			newJSONResponse(http.StatusOK, `{"success":true,"data":{"id":"chat-1"}}`),
			newJSONResponse(http.StatusOK, strings.Join([]string{
				`data: {"choices":[{"delta":{"role":"assistant","phase":"answer","status":"typing"}}]}`,
				``,
				`data: [DONE]`,
				``,
			}, "\n")),
		},
	}
	repo := &openAIAccountTestRepo{}
	account := &Account{
		ID:          7,
		Platform:    PlatformQwen,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"auth_token": "token", "cookie": "a=b"},
		Concurrency: 1,
	}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{},
	}

	err := svc.testQwenAccountConnection(ctx, account, "qwen3.6-plus", "hi")
	require.Error(t, err)
	require.Contains(t, recorder.Body.String(), `"type":"error"`)
	require.Contains(t, recorder.Body.String(), "Qwen upstream stream ended without text content")
}
