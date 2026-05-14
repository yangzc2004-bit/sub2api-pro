package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareKimiChatCompletionsRequestBody_DisablesThinkingForTools(t *testing.T) {
	body := []byte(`{"model":"kimi-k2.6-full","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup"}}]}`)

	got, err := prepareKimiChatCompletionsRequestBody(body)

	require.NoError(t, err)
	require.Equal(t, "disabled", gjson.GetBytes(got, "thinking.type").String())
}

func TestPrepareKimiChatCompletionsRequestBody_DisablesThinkingForToolHistory(t *testing.T) {
	body := []byte(`{"model":"kimi-k2.6-full","messages":[{"role":"assistant","tool_calls":[{"id":"tool_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"tool_1","content":"ok"}]}`)

	got, err := prepareKimiChatCompletionsRequestBody(body)

	require.NoError(t, err)
	require.Equal(t, "disabled", gjson.GetBytes(got, "thinking.type").String())
}

func TestPrepareKimiChatCompletionsRequestBody_PreservesExplicitThinking(t *testing.T) {
	body := []byte(`{"model":"kimi-k2.6-full","thinking":{"type":"enabled"},"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup"}}]}`)

	got, err := prepareKimiChatCompletionsRequestBody(body)

	require.NoError(t, err)
	require.Equal(t, "enabled", gjson.GetBytes(got, "thinking.type").String())
}

func TestPrepareKimiChatCompletionsRequestBody_LeavesPlainChatUnchanged(t *testing.T) {
	body := []byte(`{"model":"kimi-k2.6-full","messages":[{"role":"user","content":"hi"}]}`)

	got, err := prepareKimiChatCompletionsRequestBody(body)

	require.NoError(t, err)
	require.JSONEq(t, string(body), string(got))
	require.False(t, gjson.GetBytes(got, "thinking").Exists())
}
