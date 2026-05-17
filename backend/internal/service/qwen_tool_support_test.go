package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

// ---------------------------------------------------------------------------
// 工具名混淆 / 反混淆测试
// ---------------------------------------------------------------------------

func TestQwenToolNameObfuscation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"Read", "fs_open_file"},
		{"Write", "fs_put_file"},
		{"Edit", "fs_patch_file"},
		{"Bash", "shell_run"},
		{"write", "oc_fs_put_file"},
		{"bash", "oc_shell_run"},
		{"CustomTool", "qact_CustomTool"},
		{"qact_already", "qact_already"},
	}

	for _, tt := range tests {
		got := toQwenToolName(tt.input)
		if got != tt.expected {
			t.Errorf("toQwenToolName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestQwenToolNameDeobfuscation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"fs_open_file", "Read"},
		{"fs_put_file", "Write"},
		{"shell_run", "Bash"},
		{"oc_fs_put_file", "write"},
		{"oc_shell_run", "bash"},
		{"ReadX", "Read"},
		{"WriteX", "Write"},
		{"BashX", "Bash"},
		{"qact_CustomTool", "CustomTool"},
		{"u_CustomTool", "CustomTool"},
		{"UnknownTool", "UnknownTool"},
	}

	for _, tt := range tests {
		got := fromQwenToolName(tt.input)
		if got != tt.expected {
			t.Errorf("fromQwenToolName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestQwenToolNameRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []string{"Read", "Write", "Edit", "Bash", "CustomTool", "My_MCP_Tool"}
	for _, name := range tests {
		obfuscated := toQwenToolName(name)
		deobfuscated := fromQwenToolName(obfuscated)
		if deobfuscated != name {
			t.Errorf("round-trip failed for %q: got %q via %q", name, deobfuscated, obfuscated)
		}
	}
}

// ---------------------------------------------------------------------------
// Prompt 构建测试
// ---------------------------------------------------------------------------

func TestBuildQwenToolPromptBasic(t *testing.T) {
	t.Parallel()

	input := &qwenToolPromptInput{
		SystemPrompt: "You are a helpful assistant.",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
		Tools: []apicompat.ChatTool{},
	}

	prompt, err := buildQwenToolPrompt(input)
	if err != nil {
		t.Fatalf("buildQwenToolPrompt error: %v", err)
	}

	if !strings.Contains(prompt, "System: You are a helpful assistant.") {
		t.Error("prompt missing system message")
	}
	if !strings.Contains(prompt, "Human: Hello") {
		t.Error("prompt missing user message")
	}
	if !strings.HasSuffix(strings.TrimSpace(prompt), "Assistant:") {
		t.Error("prompt should end with Assistant:")
	}
}

func TestBuildQwenToolPromptWithTools(t *testing.T) {
	t.Parallel()

	input := &qwenToolPromptInput{
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Read file.txt"`)},
		},
		Tools: []apicompat.ChatTool{
			{
				Type: "function",
				Function: &apicompat.ChatFunction{
					Name:        "Read",
					Description: "Read a file",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}`),
				},
			},
		},
	}

	prompt, err := buildQwenToolPrompt(input)
	if err != nil {
		t.Fatalf("buildQwenToolPrompt error: %v", err)
	}

	if !strings.Contains(prompt, "CLIENT OPERATION MARKER INSTRUCTIONS") {
		t.Error("prompt missing tool instructions")
	}
	if !strings.Contains(prompt, "Available numeric choices: 1") {
		t.Error("prompt should contain numeric choice 1")
	}
	if !strings.Contains(prompt, "[[[+]]]") {
		t.Error("prompt missing operation marker format example")
	}
	if strings.Contains(prompt, "A01") || strings.Contains(prompt, "Tool X") || strings.Contains(prompt, "CLIENT_OPERATION") {
		t.Fatalf("prompt should not contain tool-looking failure examples: %s", prompt)
	}
}

func TestBuildQwenToolPromptWithShellToolUsesScriptContract(t *testing.T) {
	t.Parallel()

	input := &qwenToolPromptInput{
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Create qwen_tool_test.txt"`)},
		},
		Tools: []apicompat.ChatTool{
			{
				Type: "function",
				Function: &apicompat.ChatFunction{
					Name:        "bash",
					Description: "Run a shell command",
				},
			},
		},
	}

	prompt, err := buildQwenToolPrompt(input)
	if err != nil {
		t.Fatalf("buildQwenToolPrompt error: %v", err)
	}

	if !strings.Contains(prompt, "ADAPTER SCRIPT CONTRACT") {
		t.Fatalf("prompt missing script contract: %s", prompt)
	}
	if !strings.Contains(prompt, "```powershell") {
		t.Fatalf("prompt should require a powershell fence: %s", prompt)
	}
	if strings.Contains(prompt, "Available numeric choices") || strings.Contains(prompt, "[[[+]]]") || strings.Contains(prompt, "CLIENT_OPERATION") {
		t.Fatalf("shell prompt should avoid numeric/client-operation markers: %s", prompt)
	}
}

func TestConvertAnthropicToChatCompletionsPreservesDocumentBlocks(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model":"qwen3.6-plus",
		"max_tokens":1024,
		"messages":[{
			"role":"user",
			"content":[
				{"type":"text","text":"总结这个文档"},
				{"type":"document","title":"Sub2API部署教程.pdf","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0xLjQK"}}
			]
		}],
		"stream":true
	}`)

	got, err := ConvertAnthropicToChatCompletions(body)
	if err != nil {
		t.Fatalf("ConvertAnthropicToChatCompletions returned error: %v", err)
	}
	var req struct {
		Messages []apicompat.ChatMessage `json:"messages"`
	}
	if err := json.Unmarshal(got, &req); err != nil {
		t.Fatalf("unmarshal converted request: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(req.Messages))
	}
	var parts []map[string]any
	if err := json.Unmarshal(req.Messages[0].Content, &parts); err != nil {
		t.Fatalf("expected multipart content, got %s: %v", string(req.Messages[0].Content), err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected text and file parts, got %+v", parts)
	}
	if parts[1]["type"] != "file" || parts[1]["filename"] != "Sub2API部署教程.pdf" {
		t.Fatalf("unexpected file part: %+v", parts[1])
	}
	source, ok := parts[1]["source"].(map[string]any)
	if !ok {
		t.Fatalf("missing file source: %+v", parts[1])
	}
	if source["type"] != "base64" || source["media_type"] != "application/pdf" || source["data"] != "JVBERi0xLjQK" {
		t.Fatalf("unexpected file source: %+v", source)
	}
}

func TestBuildQwenToolPromptWithHistory(t *testing.T) {
	t.Parallel()

	input := &qwenToolPromptInput{
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Read file.txt"`)},
			{
				Role: "assistant",
				ToolCalls: []apicompat.ChatToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: apicompat.ChatFunctionCall{
							Name:      "Read",
							Arguments: `{"file_path":"file.txt"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    json.RawMessage(`"File content: hello"`),
				ToolCallID: "call_123",
			},
			{Role: "user", Content: json.RawMessage(`"Now write to it"`)},
		},
		Tools: []apicompat.ChatTool{
			{
				Type: "function",
				Function: &apicompat.ChatFunction{
					Name: "Read",
				},
			},
			{
				Type: "function",
				Function: &apicompat.ChatFunction{
					Name: "Write",
				},
			},
		},
	}

	prompt, err := buildQwenToolPrompt(input)
	if err != nil {
		t.Fatalf("buildQwenToolPrompt error: %v", err)
	}

	// 检查历史消息被正确渲染
	if !strings.Contains(prompt, "Human: Read file.txt") {
		t.Error("prompt missing first user message")
	}
	if !strings.Contains(prompt, "[[[+]]]") {
		t.Error("prompt missing operation marker from history")
	}
	if !strings.Contains(prompt, `"n":1`) {
		t.Error("prompt missing slot marker in history")
	}
	if !strings.Contains(prompt, "[Client operation result call_123]") {
		t.Error("prompt missing tool result")
	}
	if !strings.Contains(prompt, "Human: Now write to it") {
		t.Error("prompt missing latest user message")
	}
}

func TestBuildQwenToolPromptWithShellHistoryUsesScriptFence(t *testing.T) {
	t.Parallel()

	input := &qwenToolPromptInput{
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Run pwd"`)},
			{
				Role: "assistant",
				ToolCalls: []apicompat.ChatToolCall{
					{
						ID:   "call_shell",
						Type: "function",
						Function: apicompat.ChatFunctionCall{
							Name:      "bash",
							Arguments: `{"command":"Get-Location"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    json.RawMessage(`"D:\\qwen-test"`),
				ToolCallID: "call_shell",
			},
		},
		Tools: []apicompat.ChatTool{
			{Type: "function", Function: &apicompat.ChatFunction{Name: "bash", Description: "Run a shell command"}},
		},
	}

	prompt, err := buildQwenToolPrompt(input)
	if err != nil {
		t.Fatalf("buildQwenToolPrompt error: %v", err)
	}

	if !strings.Contains(prompt, "```powershell\nGet-Location\n```") {
		t.Fatalf("prompt should render shell history as a powershell fence: %s", prompt)
	}
	if strings.Contains(prompt, "[[[+]]]") {
		t.Fatalf("shell history should not render client-operation markers: %s", prompt)
	}
}

func TestCompactSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{`{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}`, "{file_path!}"},
		{`{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"number"}},"required":["a"]}`, "{a!, b}"},
		{`{"type":"object"}`, ""},
		{"", ""},
		{"invalid json", ""},
	}

	for _, tt := range tests {
		got := compactSchema(json.RawMessage(tt.input))
		if got != tt.expected {
			t.Errorf("compactSchema(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// 工具调用解析测试
// ---------------------------------------------------------------------------

func TestParseQwenToolCallsToolCallFormat(t *testing.T) {
	t.Parallel()

	text := `I'll read the file for you.

##TOOL_CALL##
{"name": "fs_open_file", "input": {"file_path": "/etc/hosts"}}
##END_CALL##`

	prefix, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %q", finishReason)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "Read" {
		t.Errorf("expected tool name Read (deobfuscated), got %q", toolCalls[0].Name)
	}
	if prefix != "I'll read the file for you." {
		t.Errorf("expected prefix %q, got %q", "I'll read the file for you.", prefix)
	}
}

func TestParseQwenToolCallsClientOperationFormat(t *testing.T) {
	t.Parallel()

	text := `[[[+]]]
{"n":1,"input":{"file_path":"qwen_tool_test.txt","content":"tool ok"}}
[[[-]]]`

	prefix, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %q", finishReason)
	}
	if prefix != "" {
		t.Fatalf("expected empty prefix, got %q", prefix)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "__qwen_slot_1" {
		t.Fatalf("expected slot tool marker, got %q", toolCalls[0].Name)
	}
	if got := toolCalls[0].Input["file_path"]; got != "qwen_tool_test.txt" {
		t.Fatalf("unexpected input: %+v", toolCalls[0].Input)
	}
}

func TestParseQwenToolCallsPowerShellFence(t *testing.T) {
	t.Parallel()

	text := "```powershell\nSet-Content -Path \"qwen_tool_test.txt\" -Value \"ok\"\n```"

	prefix, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %q", finishReason)
	}
	if prefix != "" {
		t.Fatalf("expected empty prefix, got %q", prefix)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != qwenShellToolName {
		t.Fatalf("expected shell script marker, got %q", toolCalls[0].Name)
	}
	if got := toolCalls[0].Input["command"]; got != `Set-Content -Path "qwen_tool_test.txt" -Value "ok"` {
		t.Fatalf("unexpected command: %+v", toolCalls[0].Input)
	}
}

func TestParseQwenToolCallsPowerShellTranscript(t *testing.T) {
	t.Parallel()

	text := `# 读取文件内容
$ Get-Content -Path "qwen_tool_test.txt"
tool调用成功`

	prefix, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %q", finishReason)
	}
	if prefix != "" {
		t.Fatalf("expected empty prefix, got %q", prefix)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if got := toolCalls[0].Input["command"]; got != `Get-Content -Path "qwen_tool_test.txt"` {
		t.Fatalf("unexpected command: %+v", toolCalls[0].Input)
	}
}

func TestParseQwenToolCallsMultipleTools(t *testing.T) {
	t.Parallel()

	text := `##TOOL_CALL##
{"name": "fs_open_file", "input": {"file_path": "a.txt"}}
##END_CALL##

##TOOL_CALL##
{"name": "fs_put_file", "input": {"file_path": "b.txt", "content": "hello"}}
##END_CALL##`

	_, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %q", finishReason)
	}
	if len(toolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "Read" {
		t.Errorf("expected first tool Read, got %q", toolCalls[0].Name)
	}
	if toolCalls[1].Name != "Write" {
		t.Errorf("expected second tool Write, got %q", toolCalls[1].Name)
	}
}

func TestParseQwenToolCallsXMLFormat(t *testing.T) {
	t.Parallel()

	text := `<tool_call>{"name": "shell_run", "input": {"command": "ls -la"}}</tool_call>`

	_, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %q", finishReason)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "Bash" {
		t.Errorf("expected tool name Bash, got %q", toolCalls[0].Name)
	}
}

func TestParseQwenToolCallsPlainJSON(t *testing.T) {
	t.Parallel()

	text := `{"name": "web_query", "input": {"query": "golang"}}`

	_, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %q", finishReason)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "WebSearch" {
		t.Errorf("expected tool name WebSearch, got %q", toolCalls[0].Name)
	}
}

func TestParseQwenToolCallsOpenAIToolCallsJSON(t *testing.T) {
	t.Parallel()

	text := `{"tool_calls":[{"type":"function","function":{"name":"WriteX","arguments":"{\"file_path\":\"tool_call_success.txt\",\"content\":\"tool调用成功\"}"}}]}`

	_, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %q", finishReason)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "Write" {
		t.Fatalf("expected Write tool, got %q", toolCalls[0].Name)
	}
	if got := toolCalls[0].Input["file_path"]; got != "tool_call_success.txt" {
		t.Fatalf("unexpected file_path: %+v", toolCalls[0].Input)
	}
}

func TestParseQwenToolCallsJSONStringArguments(t *testing.T) {
	t.Parallel()

	text := `{"name":"shell_run","arguments":"{\"command\":\"pwd\"}"}`

	_, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "tool_calls" || len(toolCalls) != 1 {
		t.Fatalf("expected one tool call, got %q %+v", finishReason, toolCalls)
	}
	if toolCalls[0].Name != "Bash" || toolCalls[0].Input["command"] != "pwd" {
		t.Fatalf("unexpected parsed tool call: %+v", toolCalls[0])
	}
}

func TestParseQwenToolCallsNoTools(t *testing.T) {
	t.Parallel()

	text := "This is just a normal response without any tool calls."

	prefix, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "stop" {
		t.Fatalf("expected finish_reason stop, got %q", finishReason)
	}
	if len(toolCalls) != 0 {
		t.Fatalf("expected 0 tool calls, got %d", len(toolCalls))
	}
	if prefix != text {
		t.Errorf("expected prefix to be full text, got %q", prefix)
	}
}

func TestParseQwenToolCallsDoesNotTreatPriceAsShell(t *testing.T) {
	t.Parallel()

	text := "The price is $100 today."

	prefix, toolCalls, finishReason := parseQwenToolCalls(text)
	if finishReason != "stop" {
		t.Fatalf("expected finish_reason stop, got %q", finishReason)
	}
	if len(toolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %+v", toolCalls)
	}
	if prefix != text {
		t.Fatalf("expected original text, got %q", prefix)
	}
}

// ---------------------------------------------------------------------------
// OpenAI 格式转换测试
// ---------------------------------------------------------------------------

func TestQwenToolCallsToOpenAI(t *testing.T) {
	t.Parallel()

	toolCalls := []qwenParsedToolCall{
		{
			Name:  "Read",
			Input: map[string]any{"file_path": "/etc/hosts"},
		},
		{
			Name:  "Write",
			Input: map[string]any{"file_path": "/tmp/test.txt", "content": "hello"},
		},
	}

	openaiToolCalls := qwenToolCallsToOpenAI(toolCalls)
	if len(openaiToolCalls) != 2 {
		t.Fatalf("expected 2 openai tool calls, got %d", len(openaiToolCalls))
	}

	if openaiToolCalls[0].Type != "function" {
		t.Errorf("expected type function, got %q", openaiToolCalls[0].Type)
	}
	if openaiToolCalls[0].Function.Name != "Read" {
		t.Errorf("expected function name Read, got %q", openaiToolCalls[0].Function.Name)
	}
	if openaiToolCalls[0].Function.Arguments == "" {
		t.Error("expected non-empty arguments")
	}
	if openaiToolCalls[0].ID == "" {
		t.Error("expected non-empty tool call ID")
	}

	// 检查第二个工具调用
	if openaiToolCalls[1].Function.Name != "Write" {
		t.Errorf("expected function name Write, got %q", openaiToolCalls[1].Function.Name)
	}
}

func TestQwenToolCallsToOpenAINormalizesOpenCodeToolNames(t *testing.T) {
	t.Parallel()

	tools := []apicompat.ChatTool{
		{Type: "function", Function: &apicompat.ChatFunction{Name: "write"}},
		{Type: "function", Function: &apicompat.ChatFunction{Name: "bash"}},
	}
	toolCalls := []qwenParsedToolCall{
		{Name: "u_write", Input: map[string]any{"file_path": "tool_call_success.txt", "content": "ok"}},
		{Name: "fs_put_file", Input: map[string]any{"file_path": "legacy.txt", "content": "ok"}},
		{Name: "oc_shell_run", Input: map[string]any{"command": "pwd"}},
		{Name: "A01", Input: map[string]any{"file_path": "numbered.txt", "content": "ok"}},
		{Name: "__qwen_slot_2", Input: map[string]any{"command": "echo ok"}},
	}

	openaiToolCalls := qwenToolCallsToOpenAI(toolCalls, tools)
	if len(openaiToolCalls) != 5 {
		t.Fatalf("expected 4 openai tool calls, got %d", len(openaiToolCalls))
	}
	if got := openaiToolCalls[0].Function.Name; got != "write" {
		t.Fatalf("expected u_write to normalize to write, got %q", got)
	}
	if got := openaiToolCalls[1].Function.Name; got != "write" {
		t.Fatalf("expected fs_put_file to normalize to write, got %q", got)
	}
	if got := openaiToolCalls[2].Function.Name; got != "bash" {
		t.Fatalf("expected oc_shell_run to normalize to bash, got %q", got)
	}
	if got := openaiToolCalls[3].Function.Name; got != "write" {
		t.Fatalf("expected A01 to normalize to write, got %q", got)
	}
	if got := openaiToolCalls[4].Function.Name; got != "bash" {
		t.Fatalf("expected slot 2 to normalize to bash, got %q", got)
	}
}

func TestQwenToolCallsToOpenAIDropsUnknownToolNamesWhenToolsProvided(t *testing.T) {
	t.Parallel()

	tools := []apicompat.ChatTool{
		{Type: "function", Function: &apicompat.ChatFunction{Name: "write"}},
	}
	toolCalls := []qwenParsedToolCall{
		{Name: "client_operation", Input: map[string]any{"file_path": "bad.txt"}},
		{Name: "write_file", Input: map[string]any{"file_path": "bad.txt"}},
		{Name: "__qwen_slot_1", Input: map[string]any{"file_path": "ok.txt", "content": "ok"}},
	}

	openaiToolCalls := qwenToolCallsToOpenAI(toolCalls, tools)
	if len(openaiToolCalls) != 1 {
		t.Fatalf("expected only one allowed tool call, got %d: %+v", len(openaiToolCalls), openaiToolCalls)
	}
	if got := openaiToolCalls[0].Function.Name; got != "write" {
		t.Fatalf("expected slot to normalize to write, got %q", got)
	}
}

func TestQwenToolCallsToOpenAINormalizesShellScriptMarker(t *testing.T) {
	t.Parallel()

	tools := []apicompat.ChatTool{
		{Type: "function", Function: &apicompat.ChatFunction{Name: "bash"}},
	}
	toolCalls := []qwenParsedToolCall{
		{Name: qwenShellToolName, Input: map[string]any{"command": "Get-Location"}},
	}

	openaiToolCalls := qwenToolCallsToOpenAI(toolCalls, tools)
	if len(openaiToolCalls) != 1 {
		t.Fatalf("expected one openai tool call, got %d", len(openaiToolCalls))
	}
	if got := openaiToolCalls[0].Function.Name; got != "bash" {
		t.Fatalf("expected shell marker to normalize to bash, got %q", got)
	}
	if !strings.Contains(openaiToolCalls[0].Function.Arguments, "Get-Location") {
		t.Fatalf("expected command argument, got %s", openaiToolCalls[0].Function.Arguments)
	}
	if !strings.Contains(openaiToolCalls[0].Function.Arguments, `"description"`) {
		t.Fatalf("expected bash description argument, got %s", openaiToolCalls[0].Function.Arguments)
	}
}

// ---------------------------------------------------------------------------
// 请求体解析测试
// ---------------------------------------------------------------------------

func TestParseQwenRequestBody(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		],
		"tools": [
			{"type": "function", "function": {"name": "Read", "description": "Read file"}}
		]
	}`)

	req, err := parseQwenRequestBody(body)
	if err != nil {
		t.Fatalf("parseQwenRequestBody error: %v", err)
	}

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.System != "You are helpful." {
		t.Errorf("expected system prompt 'You are helpful.', got %q", req.System)
	}
	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].Function.Name != "Read" {
		t.Errorf("expected tool name Read, got %q", req.Tools[0].Function.Name)
	}
}

// ---------------------------------------------------------------------------
// 消息文本提取测试
// ---------------------------------------------------------------------------

func TestExtractMessageText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    json.RawMessage
		expected string
	}{
		{json.RawMessage(`"hello world"`), "hello world"},
		{json.RawMessage(`[{"type":"text","text":"hello"}]`), "hello"},
		{json.RawMessage(`[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]`), "line1\nline2"},
		{json.RawMessage(``), ""},
		{nil, ""},
	}

	for _, tt := range tests {
		got := extractMessageText(tt.input)
		if got != tt.expected {
			t.Errorf("extractMessageText(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestRenderQwenHistoryMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		msg      apicompat.ChatMessage
		expected string
	}{
		{
			msg:      apicompat.ChatMessage{Role: "user", Content: json.RawMessage(`"Hello"`)},
			expected: "Human: Hello",
		},
		{
			msg:      apicompat.ChatMessage{Role: "assistant", Content: json.RawMessage(`"Hi there"`)},
			expected: "Assistant: Hi there",
		},
		{
			msg: apicompat.ChatMessage{
				Role: "assistant",
				ToolCalls: []apicompat.ChatToolCall{
					{
						Function: apicompat.ChatFunctionCall{
							Name:      "Read",
							Arguments: `{"file_path":"test.txt"}`,
						},
					},
				},
			},
			expected: "Assistant: [[[+]]]\n{\"n\":\"fs_open_file\",\"input\":{\"file_path\":\"test.txt\"}}\n[[[-]]]",
		},
		{
			msg: apicompat.ChatMessage{
				Role:       "tool",
				Content:    json.RawMessage(`"File content"`),
				ToolCallID: "call_123",
			},
			expected: "[Client operation result call_123]\nFile content\n[/Client operation result]",
		},
	}

	for _, tt := range tests {
		got := renderQwenHistoryMessage(tt.msg)
		if got != tt.expected {
			t.Errorf("renderQwenHistoryMessage(%+v) = %q, want %q", tt.msg, got, tt.expected)
		}
	}
}
