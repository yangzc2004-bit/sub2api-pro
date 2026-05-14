package service

import (
	"encoding/json"
	"testing"
)

func TestConvertAnthropicToChatCompletionsBasic(t *testing.T) {
	t.Parallel()

	anthropicBody := []byte(`{
		"model": "claude-3-opus",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": "Hello"}
		],
		"stream": true
	}`)

	openaiBody, err := ConvertAnthropicToChatCompletions(anthropicBody)
	if err != nil {
		t.Fatalf("ConvertAnthropicToChatCompletions error: %v", err)
	}

	var openaiReq map[string]interface{}
	if err := json.Unmarshal(openaiBody, &openaiReq); err != nil {
		t.Fatalf("parse openai request error: %v", err)
	}

	if openaiReq["model"] != "claude-3-opus" {
		t.Errorf("expected model claude-3-opus, got %v", openaiReq["model"])
	}
	messages, ok := openaiReq["messages"].([]interface{})
	if !ok || len(messages) != 1 {
		t.Fatalf("expected 1 message, got %v", openaiReq["messages"])
	}
	msg0, ok := messages[0].(map[string]interface{})
	if !ok || msg0["role"] != "user" {
		t.Errorf("expected role user, got %v", msg0["role"])
	}
}

func TestConvertAnthropicToChatCompletionsWithTools(t *testing.T) {
	t.Parallel()

	anthropicBody := []byte(`{
		"model": "claude-3-opus",
		"max_tokens": 1024,
		"system": "You are helpful.",
		"messages": [
			{"role": "user", "content": "Read file.txt"}
		],
		"tools": [
			{"name": "Read", "description": "Read a file", "input_schema": {"type": "object", "properties": {"file_path": {"type": "string"}}}}
		],
		"stream": false
	}`)

	openaiBody, err := ConvertAnthropicToChatCompletions(anthropicBody)
	if err != nil {
		t.Fatalf("ConvertAnthropicToChatCompletions error: %v", err)
	}

	var openaiReq map[string]interface{}
	if err := json.Unmarshal(openaiBody, &openaiReq); err != nil {
		t.Fatalf("parse openai request error: %v", err)
	}

	messages, ok := openaiReq["messages"].([]interface{})
	if !ok || len(messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %v", openaiReq["messages"])
	}

	msg0, ok := messages[0].(map[string]interface{})
	if !ok || msg0["role"] != "system" {
		t.Errorf("expected first message role system, got %v", messages[0])
	}

	tools, ok := openaiReq["tools"].([]interface{})
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", openaiReq["tools"])
	}
	tool0, ok := tools[0].(map[string]interface{})
	if !ok || tool0["type"] != "function" {
		t.Errorf("expected tool type function, got %v", tool0["type"])
	}
	fn, ok := tool0["function"].(map[string]interface{})
	if !ok || fn["name"] != "Read" {
		t.Errorf("expected tool name Read, got %v", fn["name"])
	}
}

func TestConvertAnthropicToChatCompletionsPreservesImages(t *testing.T) {
	t.Parallel()

	anthropicBody := []byte(`{
		"model": "claude-3-opus",
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": "解读这张图"},
				{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "aGVsbG8="}}
			]
		}]
	}`)

	openaiBody, err := ConvertAnthropicToChatCompletions(anthropicBody)
	if err != nil {
		t.Fatalf("ConvertAnthropicToChatCompletions error: %v", err)
	}

	var openaiReq qwenChatRequest
	if err := json.Unmarshal(openaiBody, &openaiReq); err != nil {
		t.Fatalf("parse openai request error: %v", err)
	}
	if len(openaiReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(openaiReq.Messages))
	}
	var parts []map[string]interface{}
	if err := json.Unmarshal(openaiReq.Messages[0].Content, &parts); err != nil {
		t.Fatalf("expected multimodal content parts: %v, raw=%s", err, string(openaiReq.Messages[0].Content))
	}
	if len(parts) != 2 || parts[1]["type"] != "image_url" {
		t.Fatalf("expected image_url part, got %+v", parts)
	}
	imageURL := parts[1]["image_url"].(map[string]interface{})
	if imageURL["url"] != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("unexpected image url: %+v", imageURL)
	}
}

func TestConvertAnthropicToChatCompletionsWithToolCalls(t *testing.T) {
	t.Parallel()

	anthropicBody := []byte(`{
		"model": "claude-3-opus",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": "Read file.txt"},
			{
				"role": "assistant",
				"content": [
					{"type": "text", "text": "I'll read the file."},
					{"type": "tool_use", "id": "toolu_123", "name": "Read", "input": {"file_path": "file.txt"}}
				]
			},
			{
				"role": "user",
				"content": [
					{"type": "tool_result", "tool_use_id": "toolu_123", "content": "File content here"}
				]
			}
		]
	}`)

	openaiBody, err := ConvertAnthropicToChatCompletions(anthropicBody)
	if err != nil {
		t.Fatalf("ConvertAnthropicToChatCompletions error: %v", err)
	}

	var openaiReq qwenChatRequest
	if err := json.Unmarshal(openaiBody, &openaiReq); err != nil {
		t.Fatalf("parse openai request error: %v", err)
	}

	// 应该有 3 条消息：user + assistant + tool
	//（Anthropic 的 tool_result 在 user 消息中，转换为独立的 tool 消息）
	if len(openaiReq.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(openaiReq.Messages))
	}

	// 检查 assistant 消息
	if openaiReq.Messages[1].Role != "assistant" {
		t.Errorf("expected message 1 role assistant, got %q", openaiReq.Messages[1].Role)
	}
	if len(openaiReq.Messages[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(openaiReq.Messages[1].ToolCalls))
	}
	if openaiReq.Messages[1].ToolCalls[0].Function.Name != "Read" {
		t.Errorf("expected tool call name Read, got %q", openaiReq.Messages[1].ToolCalls[0].Function.Name)
	}

	// 检查 tool 消息
	if openaiReq.Messages[2].Role != "tool" {
		t.Errorf("expected message 2 role tool, got %q", openaiReq.Messages[2].Role)
	}
	if openaiReq.Messages[2].ToolCallID != "toolu_123" {
		t.Errorf("expected tool_call_id toolu_123, got %q", openaiReq.Messages[2].ToolCallID)
	}
}

func TestConvertAnthropicToChatCompletionsWithSystemArray(t *testing.T) {
	t.Parallel()

	anthropicBody := []byte(`{
		"model": "claude-3-opus",
		"system": [
			{"type": "text", "text": "You are helpful."},
			{"type": "text", "text": "Be concise."}
		],
		"messages": [
			{"role": "user", "content": "Hello"}
		]
	}`)

	openaiBody, err := ConvertAnthropicToChatCompletions(anthropicBody)
	if err != nil {
		t.Fatalf("ConvertAnthropicToChatCompletions error: %v", err)
	}

	var openaiReq map[string]interface{}
	if err := json.Unmarshal(openaiBody, &openaiReq); err != nil {
		t.Fatalf("parse openai request error: %v", err)
	}

	messages, ok := openaiReq["messages"].([]interface{})
	if !ok || len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %v", openaiReq["messages"])
	}
	msg0, ok := messages[0].(map[string]interface{})
	if !ok || msg0["role"] != "system" {
		t.Errorf("expected first message role system, got %v", messages[0])
	}
	// 检查 system 内容是否合并
	var qwenReq qwenChatRequest
	json.Unmarshal(openaiBody, &qwenReq)
	systemText := extractMessageText(qwenReq.Messages[0].Content)
	if systemText != "You are helpful.\nBe concise." {
		t.Errorf("unexpected system content: %q", systemText)
	}
}

// TestConvertAnthropicToChatCompletionsRoundTrip 验证简单对话能正确转换。
func TestConvertAnthropicToChatCompletionsRoundTrip(t *testing.T) {
	t.Parallel()

	// 原始 Anthropic 请求
	anthropicReq := map[string]interface{}{
		"model":      "claude-sonnet-4-6",
		"max_tokens": 8192,
		"stream":     true,
		"system":     "You are Claude Code",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "List files"},
			{
				"role": "assistant",
				"content": []map[string]interface{}{
					{"type": "text", "text": "I'll list the files."},
					{
						"type":  "tool_use",
						"id":    "toolu_01",
						"name":  "Bash",
						"input": map[string]string{"command": "ls -la"},
					},
				},
			},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": "toolu_01",
						"content":     "total 10\ndrwx...",
					},
				},
			},
		},
		"tools": []map[string]interface{}{
			{
				"name":         "Bash",
				"description":  "Run shell commands",
				"input_schema": map[string]string{"type": "object"},
			},
		},
	}

	anthropicBody, _ := json.Marshal(anthropicReq)
	openaiBody, err := ConvertAnthropicToChatCompletions(anthropicBody)
	if err != nil {
		t.Fatalf("ConvertAnthropicToChatCompletions error: %v", err)
	}

	// 验证 OpenAI 格式
	var result map[string]interface{}
	if err := json.Unmarshal(openaiBody, &result); err != nil {
		t.Fatalf("unmarshal result error: %v", err)
	}

	if result["model"] != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %v", result["model"])
	}
	if result["stream"] != true {
		t.Errorf("expected stream true, got %v", result["stream"])
	}

	messages, ok := result["messages"].([]interface{})
	if !ok {
		t.Fatal("expected messages to be array")
	}
	if len(messages) != 4 { // system + user + assistant + tool
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}

	// 验证 tool 消息
	toolMsg, ok := messages[3].(map[string]interface{})
	if !ok || toolMsg["role"] != "tool" {
		t.Errorf("expected message 3 to be tool, got %+v", messages[3])
	}
}
