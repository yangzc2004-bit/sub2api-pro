package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// 工具名混淆 / 反混淆
// ---------------------------------------------------------------------------

// qwenToolAliasMap 定义常见工具名的别名映射。
// Qwen 上游会把常见短名（Read/Write/Bash/Edit…）当内置函数校验并返回
// "Tool X does not exists."。通过别名绕过该校验。
var qwenToolAliasMap = map[string]string{
	"Read":         "fs_open_file",
	"Write":        "fs_put_file",
	"Edit":         "fs_patch_file",
	"Bash":         "shell_run",
	"Glob":         "path_find",
	"Grep":         "text_search",
	"WebFetch":     "http_get_url",
	"WebSearch":    "web_query",
	"NotebookEdit": "notebook_patch",

	"read":       "oc_fs_open_file",
	"write":      "oc_fs_put_file",
	"edit":       "oc_fs_patch_file",
	"bash":       "oc_shell_run",
	"glob":       "oc_path_find",
	"grep":       "oc_text_search",
	"list":       "oc_path_list",
	"webfetch":   "oc_http_get_url",
	"web_search": "oc_web_query",
	"websearch":  "oc_web_lookup",
	"todowrite":  "oc_task_list_write",
	"todoread":   "oc_task_list_read",
}

var qwenLegacyToolAliasReverse = map[string]string{
	"ReadX":            "Read",
	"WriteX":           "Write",
	"EditX":            "Edit",
	"BashX":            "Bash",
	"GlobX":            "Glob",
	"GrepX":            "Grep",
	"WebFetchX":        "WebFetch",
	"WebSearchX":       "WebSearch",
	"AgentX":           "Agent",
	"TaskCreateX":      "TaskCreate",
	"TaskUpdateX":      "TaskUpdate",
	"AskUserQuestionX": "AskUserQuestion",
	"NotebookEditX":    "NotebookEdit",
}

const (
	qwenGenericToolPrefix = "qact_"
	qwenClientSlotPrefix  = "__qwen_slot_"
	qwenShellToolName     = "__qwen_shell_script"
	qwenClientBlockStart  = "[[[+]]]"
	qwenClientBlockEnd    = "[[[-]]]"
)

func qwenClientSlotForIndex(index int) string {
	return fmt.Sprintf("%s%d", qwenClientSlotPrefix, index+1)
}

func qwenClientSlotToIndex(name string) (int, bool) {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, qwenClientSlotPrefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(name, qwenClientSlotPrefix))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n - 1, true
}

func qwenSlotValueToIndex(value interface{}) (int, bool) {
	switch v := value.(type) {
	case float64:
		n := int(v)
		if float64(n) != v || n <= 0 {
			return 0, false
		}
		return n - 1, true
	case int:
		if v <= 0 {
			return 0, false
		}
		return v - 1, true
	case string:
		raw := strings.TrimSpace(v)
		if idx, ok := qwenClientSlotToIndex(raw); ok {
			return idx, true
		}
		raw = strings.TrimPrefix(raw, "#")
		raw = strings.TrimPrefix(strings.ToLower(raw), "slot")
		raw = strings.TrimPrefix(raw, "n")
		raw = strings.Trim(raw, " _:-")
		if idx, ok := qwenActionAliasToIndex(raw); ok {
			return idx, true
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return 0, false
		}
		return n - 1, true
	default:
		return 0, false
	}
}

func qwenActionAliasForIndex(index int) string {
	return fmt.Sprintf("A%02d", index+1)
}

func qwenActionAliasToIndex(name string) (int, bool) {
	name = strings.TrimSpace(strings.ToUpper(name))
	if len(name) < 2 || name[0] != 'A' {
		return 0, false
	}
	n, err := strconv.Atoi(name[1:])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n - 1, true
}

// qwenToolAliasReverse 反向映射（别名 -> 原名）
var qwenToolAliasReverse map[string]string

func init() {
	qwenToolAliasReverse = make(map[string]string, len(qwenToolAliasMap)+len(qwenLegacyToolAliasReverse))
	for k, v := range qwenToolAliasMap {
		qwenToolAliasReverse[v] = k
	}
	for k, v := range qwenLegacyToolAliasReverse {
		if _, exists := qwenToolAliasReverse[k]; !exists {
			qwenToolAliasReverse[k] = v
		}
	}
}

// toQwenToolName 出站混淆：把客户端工具名替换成 Qwen-safe 别名。
func toQwenToolName(name string) string {
	if alias, ok := qwenToolAliasMap[name]; ok {
		return alias
	}
	// 通用兜底：其余客户端工具自动加 u_ 前缀
	if !strings.HasPrefix(name, qwenGenericToolPrefix) && len(name) > 0 {
		return qwenGenericToolPrefix + name
	}
	return name
}

// fromQwenToolName 入站反混淆：把 Qwen 返回的别名还原为客户端原名。
func fromQwenToolName(name string) string {
	if orig, ok := qwenToolAliasReverse[name]; ok {
		return orig
	}
	// 去掉通用 u_ 前缀
	if strings.HasPrefix(name, qwenGenericToolPrefix) {
		return name[len(qwenGenericToolPrefix):]
	}
	if strings.HasPrefix(name, "u_") {
		return name[2:]
	}
	return name
}

// ---------------------------------------------------------------------------
// Prompt 构建
// ---------------------------------------------------------------------------

// qwenToolPromptInput 构建 Qwen prompt 所需的输入。
type qwenToolPromptInput struct {
	SystemPrompt string
	Messages     []apicompat.ChatMessage
	Tools        []apicompat.ChatTool
}

// buildQwenToolPrompt 将 OpenAI 格式的 messages + tools 转换为 Qwen 网页版能理解的 prompt。
// 参考 qwen2API 的 prompt_builder.py 核心逻辑，但做了简化以适配当前项目架构。
func buildQwenToolPrompt(input *qwenToolPromptInput) (string, error) {
	var parts []string

	// 1. system prompt
	if input.SystemPrompt != "" {
		parts = append(parts, fmt.Sprintf("System: %s", sanitizeQwenPromptText(input.SystemPrompt, input.Tools)))
	}

	// 2. tools instruction block（仅当存在 tools 时）
	if len(input.Tools) > 0 {
		toolBlock := buildQwenToolInstructionBlock(input.Tools)
		parts = append(parts, toolBlock)
	}

	// 3. 历史消息转换
	for _, msg := range input.Messages {
		if msg.Role == "system" {
			continue // system 已单独处理
		}
		line := renderQwenHistoryMessageWithTools(msg, input.Tools)
		if line != "" {
			parts = append(parts, line)
		}
	}

	// 4. 最后必须是 Assistant: 提示模型回复
	parts = append(parts, "Assistant:")
	return strings.Join(parts, "\n\n"), nil
}

// buildQwenToolInstructionBlock 构建工具指令块。
func buildQwenToolInstructionBlock(tools []apicompat.ChatTool) string {
	if qwenShellToolAvailable(tools) {
		return buildQwenShellScriptInstructionBlock()
	}
	return buildQwenActionInstructionBlock(tools)

	var lines []string
	lines = append(lines, "=== MANDATORY TOOL CALL INSTRUCTIONS ===")
	lines = append(lines, "【重要】用户输入什么语言，就用什么语言回复。")
	lines = append(lines, "")

	// 收集工具名（已混淆）
	var names []string
	for _, t := range tools {
		if t.Function != nil && t.Function.Name != "" {
			names = append(names, toQwenToolName(t.Function.Name))
		}
	}
	lines = append(lines, fmt.Sprintf("You have access to these tools: %s", strings.Join(names, ", ")))
	lines = append(lines, "")
	lines = append(lines, "Use tools only when they are necessary to directly answer the CURRENT TASK.")
	lines = append(lines, "If you already know the answer, answer directly without any tool call.")
	lines = append(lines, "Follow the current platform tool contract exactly.")
	lines = append(lines, "Do not drift into Qwen-native or builtin tool-call formats, wrappers, tags, or argument schemas.")
	lines = append(lines, "When the user asks to create, edit, read, run, search, fetch, or otherwise operate on local resources, emit a tool call instead of saying you cannot access the filesystem or environment.")
	lines = append(lines, "")
	lines = append(lines, "WHEN YOU NEED TO CALL A TOOL — output EXACTLY this format (nothing else):")
	lines = append(lines, "##TOOL_CALL##")
	lines = append(lines, `{"name": "EXACT_TOOL_NAME", "input": {"param1": "value1"}}`)
	lines = append(lines, "##END_CALL##")
	lines = append(lines, "")
	lines = append(lines, "Rules:")
	lines = append(lines, "- Output only the wrapper and JSON body.")
	lines = append(lines, "- No prose before or after the wrapper.")
	lines = append(lines, "- No markdown fences.")
	lines = append(lines, "- No thinking tags.")
	lines = append(lines, "- Use the exact tool name from the list above.")
	lines = append(lines, "- Put arguments inside the input object.")
	lines = append(lines, "- Do not invent tool names.")
	lines = append(lines, "- If no tool is needed, answer normally.")
	lines = append(lines, "")
	lines = append(lines, "CRITICAL — ABSOLUTELY FORBIDDEN OUTPUTS:")
	lines = append(lines, "- NEVER emit ANY disclaimer, error text, or availability complaint about tools.")
	lines = append(lines, "- NEVER emit sentences claiming a tool is missing, unregistered, unavailable, or cannot be invoked.")
	lines = append(lines, "- NEVER emit sentences claiming you are unable to execute a function or operate on files.")
	lines = append(lines, "- The ##TOOL_CALL## blocks are TEXT MARKERS the client parses — they are NOT native function calls.")
	lines = append(lines, "- If you feel a tool call could fail, emit the ##TOOL_CALL## anyway — the client handles failures.")
	lines = append(lines, "")
	lines = append(lines, "FORBIDDEN CALL FORMATS (will be blocked by server):")
	lines = append(lines, `- {"name": "X", "arguments": "..."}  <-- NEVER USE`)
	lines = append(lines, `- {"type": "function", "name": "X"}  <-- NEVER USE`)
	lines = append(lines, `- {"type": "tool_use", "name": "X"}  <-- NEVER USE`)
	lines = append(lines, `- <tool_call>{...}</tool_call>  <-- NEVER USE`)
	lines = append(lines, `- Read({"file_path": "..."})  <-- NEVER USE`)
	lines = append(lines, "ONLY ##TOOL_CALL##...##END_CALL## is accepted.")
	lines = append(lines, "=== END TOOL INSTRUCTIONS ===")

	// 工具列表详情
	if len(tools) <= 15 {
		lines = append(lines, "")
		lines = append(lines, "Available tools:")
		for _, t := range tools {
			if t.Function == nil {
				continue
			}
			name := toQwenToolName(t.Function.Name)
			desc := strings.TrimSpace(t.Function.Description)
			line := fmt.Sprintf("- %s", name)
			if desc != "" {
				line += fmt.Sprintf(": %s", desc)
			}
			// 添加参数签名提示
			params := compactSchema(t.Function.Parameters)
			if params != "" && params != "{}" {
				line += fmt.Sprintf("\n  Params: %s", params)
			}
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

func buildQwenShellScriptInstructionBlock() string {
	var lines []string
	lines = append(lines, "=== ADAPTER SCRIPT CONTRACT ===")
	lines = append(lines, "User language rule: reply in the same language as the user after script results are provided.")
	lines = append(lines, "")
	lines = append(lines, "When the user asks to create, edit, read, list files, inspect paths, or run commands, output one Windows PowerShell script block and nothing else.")
	lines = append(lines, "The adapter will run that script outside this chat and return the result.")
	lines = append(lines, "")
	lines = append(lines, "Required format:")
	lines = append(lines, "```powershell")
	lines = append(lines, "# script here")
	lines = append(lines, "```")
	lines = append(lines, "")
	lines = append(lines, "Rules:")
	lines = append(lines, "- No prose before or after the script block.")
	lines = append(lines, "- Put runnable PowerShell commands only inside the script block; do not prefix commands with shell prompts like $.")
	lines = append(lines, "- Do not include guessed command output. The adapter will provide the real output.")
	lines = append(lines, "- Do not emit availability complaints or platform error text.")
	lines = append(lines, "- Do not use any native invocation syntax.")
	lines = append(lines, "- For non-ASCII paths or text, prefer .NET file APIs with UTF-8, for example [System.IO.File]::WriteAllText and [System.IO.File]::ReadAllText.")
	lines = append(lines, "- Prefer paths relative to the current directory unless the user gives an absolute path.")
	lines = append(lines, "=== END ADAPTER SCRIPT CONTRACT ===")
	return strings.Join(lines, "\n")
}

func buildQwenActionInstructionBlock(tools []apicompat.ChatTool) string {
	var lines []string
	lines = append(lines, "=== CLIENT OPERATION MARKER INSTRUCTIONS ===")
	lines = append(lines, "User language rule: reply in the same language as the user.")
	lines = append(lines, "")

	var slots []string
	for i, t := range tools {
		if t.Function != nil && t.Function.Name != "" {
			slots = append(slots, strconv.Itoa(i+1))
		}
	}
	lines = append(lines, fmt.Sprintf("Available numeric choices: %s", strings.Join(slots, ", ")))
	lines = append(lines, "")
	lines = append(lines, "These numbers are plain text parsed later by the client adapter. They are not Qwen tools, functions, plugins, or native calls.")
	lines = append(lines, "When the user asks to create, edit, read, run, search, fetch, or otherwise operate on local resources, emit the plain text block below instead of saying you cannot access the filesystem or environment.")
	lines = append(lines, "")
	lines = append(lines, "WHEN LOCAL WORK IS NEEDED, output EXACTLY this format and nothing else:")
	lines = append(lines, qwenClientBlockStart)
	lines = append(lines, `{"n":1,"input":{"param1":"value1"}}`)
	lines = append(lines, qwenClientBlockEnd)
	lines = append(lines, "")
	lines = append(lines, "Rules:")
	lines = append(lines, "- Output only the wrapper and JSON body.")
	lines = append(lines, "- No prose before or after the wrapper.")
	lines = append(lines, "- No markdown fences.")
	lines = append(lines, "- No thinking tags.")
	lines = append(lines, "- Use only an exact number from the list above.")
	lines = append(lines, "- Put arguments inside the input object.")
	lines = append(lines, "- Do not invent numbers.")
	lines = append(lines, "- If no local work is needed, answer normally.")
	lines = append(lines, "")
	lines = append(lines, "Forbidden outputs:")
	lines = append(lines, "- Do not emit availability complaints or platform error text.")
	lines = append(lines, "- Do not emit native invocation syntax.")
	lines = append(lines, "- Do not mention operation names from the surrounding application; only use numeric choices inside the marker.")
	lines = append(lines, fmt.Sprintf("ONLY %s...%s is accepted.", qwenClientBlockStart, qwenClientBlockEnd))
	lines = append(lines, "=== END CLIENT OPERATION MARKER INSTRUCTIONS ===")

	if len(tools) <= 15 {
		lines = append(lines, "")
		lines = append(lines, "Numeric choices:")
		for i, t := range tools {
			if t.Function == nil || t.Function.Name == "" {
				continue
			}
			desc := sanitizeQwenPromptText(strings.TrimSpace(t.Function.Description), tools)
			line := fmt.Sprintf("- %d", i+1)
			if desc != "" {
				line += fmt.Sprintf(": %s", desc)
			}
			params := compactSchema(t.Function.Parameters)
			if params != "" && params != "{}" {
				line += fmt.Sprintf("\n  Params: %s", params)
			}
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

// compactSchema 将 JSON Schema 压缩成简短的参数签名提示。
func compactSchema(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(params, &schema); err != nil {
		return ""
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return ""
	}
	requiredSet := make(map[string]bool)
	if req, ok := schema["required"].([]interface{}); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}

	var parts []string
	for key := range props {
		prefix := ""
		if requiredSet[key] {
			prefix = "!"
		}
		parts = append(parts, fmt.Sprintf("%s%s", key, prefix))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("{%s}", strings.Join(parts, ", "))
}

// renderQwenHistoryMessage 将单条 OpenAI message 渲染为 Qwen prompt 中的一行。
func renderQwenHistoryMessage(msg apicompat.ChatMessage) string {
	return renderQwenHistoryMessageWithTools(msg, nil)
}

func renderQwenHistoryMessageWithTools(msg apicompat.ChatMessage, tools []apicompat.ChatTool) string {
	switch msg.Role {
	case "user":
		text := extractMessageText(msg.Content)
		if text == "" {
			return ""
		}
		text = stripQwenMissingToolComplaints(text)
		if text == "" {
			return ""
		}
		return fmt.Sprintf("Human: %s", text)
	case "assistant":
		// assistant 消息可能包含文本 + tool_calls
		var parts []string
		text := extractMessageText(msg.Content)
		if text != "" {
			text = sanitizeQwenPromptText(text, tools)
		}
		if text != "" {
			parts = append(parts, text)
		}
		// 渲染 tool_calls 为 ##TOOL_CALL## 格式
		shellName := qwenFindShellToolName(tools)
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name == "" {
				continue
			}
			args := tc.Function.Arguments
			if args == "" {
				args = "{}"
			}
			if qwenToolNameMatches(tc.Function.Name, shellName) {
				if command := qwenShellCommandFromArguments(args); command != "" {
					parts = append(parts, fmt.Sprintf("```powershell\n%s\n```", command))
					continue
				}
			}
			payload := ""
			if slot := qwenClientSlotForToolName(tc.Function.Name, tools); slot != "" {
				payload = fmt.Sprintf(`{"n":%s,"input":%s}`, slot, args)
			} else {
				payload = fmt.Sprintf(`{"n":%q,"input":%s}`, toQwenToolName(tc.Function.Name), args)
			}
			parts = append(parts, fmt.Sprintf("%s\n%s\n%s", qwenClientBlockStart, payload, qwenClientBlockEnd))
		}
		if len(parts) == 0 {
			return ""
		}
		return fmt.Sprintf("Assistant: %s", strings.Join(parts, "\n"))
	case "tool":
		// tool result
		text := extractMessageText(msg.Content)
		if text == "" {
			return ""
		}
		text = sanitizeQwenPromptText(text, tools)
		if text == "" {
			return ""
		}
		toolCallID := msg.ToolCallID
		if toolCallID == "" {
			toolCallID = "unknown"
		}
		return fmt.Sprintf("[Client operation result %s]\n%s\n[/Client operation result]", toolCallID, text)
	default:
		return ""
	}
}

func qwenActionAliasForToolName(name string, tools []apicompat.ChatTool) string {
	if name == "" {
		return ""
	}
	for i, t := range tools {
		if t.Function != nil && t.Function.Name == name {
			return qwenActionAliasForIndex(i)
		}
	}
	return ""
}

func qwenClientSlotForToolName(name string, tools []apicompat.ChatTool) string {
	if name == "" {
		return ""
	}
	for i, t := range tools {
		if t.Function != nil && t.Function.Name == name {
			return strconv.Itoa(i + 1)
		}
	}
	return ""
}

func qwenShellToolAvailable(tools []apicompat.ChatTool) bool {
	return qwenFindShellToolName(tools) != ""
}

func qwenFindShellToolName(tools []apicompat.ChatTool) string {
	for _, t := range tools {
		if t.Function == nil || t.Function.Name == "" {
			continue
		}
		if qwenToolAliasKey(t.Function.Name) == "bash" ||
			qwenToolAliasKey(t.Function.Name) == "shell" ||
			qwenToolAliasKey(t.Function.Name) == "shellrun" ||
			qwenToolAliasKey(t.Function.Name) == "runcommand" {
			return t.Function.Name
		}
	}
	for _, t := range tools {
		if t.Function == nil || t.Function.Name == "" {
			continue
		}
		desc := strings.ToLower(t.Function.Description)
		if strings.Contains(desc, "shell") || strings.Contains(desc, "command") || strings.Contains(desc, "powershell") || strings.Contains(desc, "bash") {
			return t.Function.Name
		}
	}
	return ""
}

func qwenToolNameMatches(name, target string) bool {
	if name == "" || target == "" {
		return false
	}
	if name == target {
		return true
	}
	nameKey := qwenToolAliasKey(name)
	targetKey := qwenToolAliasKey(target)
	if nameKey != "" && nameKey == targetKey {
		return true
	}
	deobfuscatedKey := qwenToolAliasKey(fromQwenToolName(name))
	return deobfuscatedKey != "" && deobfuscatedKey == targetKey
}

func qwenShellCommandFromArguments(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(args), &obj); err == nil {
		for _, key := range []string{"command", "cmd", "script"} {
			if value, ok := obj[key].(string); ok {
				if value = strings.TrimSpace(value); value != "" {
					return value
				}
			}
		}
		if len(obj) == 1 {
			for _, value := range obj {
				if text, ok := value.(string); ok {
					if text = strings.TrimSpace(text); text != "" {
						return text
					}
				}
			}
		}
	}
	return args
}

func sanitizeQwenPromptText(text string, tools []apicompat.ChatTool) string {
	text = stripQwenMissingToolComplaints(text)
	if text == "" {
		return ""
	}
	if len(tools) == 0 {
		return text
	}
	for _, t := range tools {
		if t.Function == nil || t.Function.Name == "" {
			continue
		}
		text = qwenMaskToolNameInText(text, t.Function.Name)
		if alias := toQwenToolName(t.Function.Name); alias != t.Function.Name {
			text = qwenMaskToolNameInText(text, alias)
		}
	}
	for alias := range qwenToolAliasReverse {
		text = qwenMaskToolNameInText(text, alias)
	}
	return text
}

func stripQwenMissingToolComplaints(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if qwenLooksLikeMissingToolComplaint(trimmed) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func qwenLooksLikeMissingToolComplaint(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(line, "\u5de5\u5177") && (strings.Contains(line, "\u4e0d\u5b58\u5728") || strings.Contains(line, "\u4e0d\u53ef\u7528")) {
		return true
	}
	if strings.Contains(line, "\u65e0\u6cd5") && (strings.Contains(line, "\u521b\u5efa\u6587\u4ef6") || strings.Contains(line, "\u5de5\u5177")) {
		return true
	}
	if strings.Contains(lower, "tool ") && (strings.Contains(lower, "does not exist") || strings.Contains(lower, "does not exists")) {
		return true
	}
	if strings.Contains(lower, "工具") && (strings.Contains(lower, "不存在") || strings.Contains(lower, "不可用")) {
		return true
	}
	if strings.Contains(lower, "无法") && (strings.Contains(lower, "创建文件") || strings.Contains(lower, "工具")) {
		return true
	}
	return false
}

func qwenMaskToolNameInText(text, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return text
	}
	escaped := regexp.QuoteMeta(name)
	re := regexp.MustCompile(`(?i)\b` + escaped + `\b`)
	return re.ReplaceAllString(text, "client operation")
}

// extractMessageText 从 ChatMessage.Content (json.RawMessage) 中提取纯文本。
func extractMessageText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	// 尝试作为字符串解析
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	// 尝试作为数组解析（OpenAI content parts）
	var parts []map[string]interface{}
	if err := json.Unmarshal(content, &parts); err == nil {
		var texts []string
		for _, p := range parts {
			partType, _ := p["type"].(string)
			switch strings.ToLower(strings.TrimSpace(partType)) {
			case "text", "input_text", "output_text", "":
				if txt, ok := p["text"].(string); ok {
					texts = append(texts, txt)
				}
			case "image_url", "input_image", "image":
				texts = append(texts, "[Image attached]")
			}
		}
		return strings.Join(texts, "\n")
	}
	// 兜底：直接转为字符串
	return string(content)
}

// ---------------------------------------------------------------------------
// 工具调用解析
// ---------------------------------------------------------------------------

var (
	// 匹配 ##TOOL_CALL## ... ##END_CALL## 格式（支持多行）
	qwenToolCallBlockRegex   = regexp.MustCompile(`(?s)##TOOL_CALL##\s*(.+?)\s*##END_CALL##`)
	qwenActionBlockRegex     = regexp.MustCompile(`(?s)##ACTION##\s*(.+?)\s*##END_ACTION##`)
	qwenClientOpBlockRegex   = regexp.MustCompile(`(?s)\[\[\[\+\]\]\]\s*(.+?)\s*\[\[\[-\]\]\]`)
	qwenPowerShellFenceRegex = regexp.MustCompile("(?is)```(?:powershell|pwsh|ps1|ps)\\s*\\n(.+?)\\n```")
	// 匹配 <tool_call> ... </tool_call> XML 格式
	qwenXMLToolCallRegex = regexp.MustCompile(`(?si)<tool_call>\s*(.+?)\s*</tool_call>`)
	// 匹配 ```tool_call ... ``` 代码块格式
	qwenCodeBlockToolCallRegex = regexp.MustCompile("(?s)```tool_call\\s*\\n(.+?)\\n```")
)

// qwenParsedToolCall 表示从 Qwen 响应中解析出的工具调用。
type qwenParsedToolCall struct {
	Name  string
	Input map[string]interface{}
}

// parseQwenToolCalls 从 Qwen 的文本响应中解析工具调用。
// 返回 (prefixText, toolCalls, finishReason)
func parseQwenToolCalls(text string) (string, []qwenParsedToolCall, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil, "stop"
	}

	if matches := qwenPowerShellFenceRegex.FindAllStringSubmatchIndex(text, -1); len(matches) > 0 {
		var toolCalls []qwenParsedToolCall
		for _, m := range matches {
			script := strings.TrimSpace(text[m[2]:m[3]])
			if script != "" {
				toolCalls = append(toolCalls, qwenParsedToolCall{
					Name:  qwenShellToolName,
					Input: map[string]interface{}{"command": script},
				})
			}
		}
		prefix := strings.TrimSpace(text[:matches[0][0]])
		if len(toolCalls) > 0 {
			return prefix, toolCalls, "tool_calls"
		}
	}

	if prefix, script, ok := qwenExtractPowerShellTranscript(text); ok {
		return prefix, []qwenParsedToolCall{{
			Name:  qwenShellToolName,
			Input: map[string]interface{}{"command": script},
		}}, "tool_calls"
	}

	if matches := qwenClientOpBlockRegex.FindAllStringSubmatchIndex(text, -1); len(matches) > 0 {
		var toolCalls []qwenParsedToolCall
		for _, m := range matches {
			jsonStr := strings.TrimSpace(text[m[2]:m[3]])
			if tc := parseToolCallJSON(jsonStr); tc != nil {
				toolCalls = append(toolCalls, *tc)
			}
		}
		prefix := strings.TrimSpace(text[:matches[0][0]])
		if len(toolCalls) > 0 {
			return prefix, toolCalls, "tool_calls"
		}
	}

	if matches := qwenActionBlockRegex.FindAllStringSubmatchIndex(text, -1); len(matches) > 0 {
		var toolCalls []qwenParsedToolCall
		for _, m := range matches {
			jsonStr := strings.TrimSpace(text[m[2]:m[3]])
			if tc := parseToolCallJSON(jsonStr); tc != nil {
				toolCalls = append(toolCalls, *tc)
			}
		}
		prefix := strings.TrimSpace(text[:matches[0][0]])
		if len(toolCalls) > 0 {
			return prefix, toolCalls, "tool_calls"
		}
	}

	// 1. 优先匹配 ##TOOL_CALL## ... ##END_CALL## 格式
	matches := qwenToolCallBlockRegex.FindAllStringSubmatchIndex(text, -1)
	if len(matches) > 0 {
		var toolCalls []qwenParsedToolCall
		lastEnd := 0
		for _, m := range matches {
			if m[0] > lastEnd {
				lastEnd = m[0]
			}
			jsonStr := strings.TrimSpace(text[m[2]:m[3]])
			if tc := parseToolCallJSON(jsonStr); tc != nil {
				toolCalls = append(toolCalls, *tc)
			}
		}
		prefix := strings.TrimSpace(text[:matches[0][0]])
		if len(toolCalls) > 0 {
			return prefix, toolCalls, "tool_calls"
		}
	}

	// 2. 匹配 XML <tool_call> 格式
	if xmlMatches := qwenXMLToolCallRegex.FindAllStringSubmatchIndex(text, -1); len(xmlMatches) > 0 {
		var toolCalls []qwenParsedToolCall
		for _, m := range xmlMatches {
			jsonStr := strings.TrimSpace(text[m[2]:m[3]])
			if tc := parseToolCallJSON(jsonStr); tc != nil {
				toolCalls = append(toolCalls, *tc)
			}
		}
		prefix := strings.TrimSpace(text[:xmlMatches[0][0]])
		if len(toolCalls) > 0 {
			return prefix, toolCalls, "tool_calls"
		}
	}

	// 3. 匹配 ```tool_call 代码块格式
	if cbMatches := qwenCodeBlockToolCallRegex.FindAllStringSubmatchIndex(text, -1); len(cbMatches) > 0 {
		var toolCalls []qwenParsedToolCall
		for _, m := range cbMatches {
			jsonStr := strings.TrimSpace(text[m[2]:m[3]])
			if tc := parseToolCallJSON(jsonStr); tc != nil {
				toolCalls = append(toolCalls, *tc)
			}
		}
		prefix := strings.TrimSpace(text[:cbMatches[0][0]])
		if len(toolCalls) > 0 {
			return prefix, toolCalls, "tool_calls"
		}
	}

	// 4. 尝试匹配纯 JSON 对象（整个回复就是一个工具调用）
	if strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}") {
		if toolCalls := parseToolCallJSONList(text); len(toolCalls) > 0 {
			return "", toolCalls, "tool_calls"
		}
	}

	// 无工具调用，作为普通文本
	return text, nil, "stop"
}

func qwenExtractPowerShellTranscript(text string) (string, string, bool) {
	normalized := strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if normalized == "" {
		return "", "", false
	}

	var prefixLines []string
	var commands []string
	foundCommand := false
	for _, line := range strings.Split(normalized, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		command := ""
		if strings.HasPrefix(trimmed, "$ ") {
			command = strings.TrimSpace(strings.TrimPrefix(trimmed, "$ "))
		} else if strings.HasPrefix(trimmed, "$") {
			command = trimmed
		}
		if command != "" && qwenLooksLikePowerShellCommand(command) {
			foundCommand = true
			commands = append(commands, command)
			continue
		}
		if !foundCommand && !strings.HasPrefix(trimmed, "#") && !qwenLooksLikeMissingToolComplaint(trimmed) {
			prefixLines = append(prefixLines, line)
		}
	}
	if len(commands) == 0 {
		return "", "", false
	}
	prefix := strings.TrimSpace(stripQwenMissingToolComplaints(strings.Join(prefixLines, "\n")))
	return prefix, strings.Join(commands, "\n"), true
}

func qwenLooksLikePowerShellCommand(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "[system.") {
		return true
	}
	if strings.HasPrefix(lower, "$") {
		return strings.Contains(lower, "=") ||
			strings.Contains(lower, "|") ||
			strings.Contains(lower, ".") ||
			strings.Contains(lower, ":") ||
			strings.Contains(lower, "(")
	}
	prefixes := []string{
		"set-content", "get-content", "get-childitem", "get-location",
		"new-item", "remove-item", "copy-item", "move-item", "test-path",
		"select-object", "sort-object", "where-object", "write-output",
		"write-host", "python", "python3", "py ", "node ", "npm ",
		"go ", "git ", "cmd ", "powershell ", "pwsh ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// parseToolCallJSON 解析单条工具调用 JSON。
func parseToolCallJSON(jsonStr string) *qwenParsedToolCall {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return nil
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
		return nil
	}
	if calls := qwenToolCallsFromObject(obj); len(calls) > 0 {
		return &calls[0]
	}

	name := ""
	if n, ok := obj["name"].(string); ok {
		name = n
	} else if n, ok := obj["action"].(string); ok {
		name = n
	} else if idx, ok := qwenSlotValueToIndex(obj["n"]); ok {
		name = qwenClientSlotForIndex(idx)
	} else if idx, ok := qwenSlotValueToIndex(obj["slot"]); ok {
		name = qwenClientSlotForIndex(idx)
	} else if n, ok := obj["function"].(map[string]interface{}); ok {
		if fn, ok := n["name"].(string); ok {
			name = fn
		}
	}
	if name == "" {
		return nil
	}

	var input map[string]interface{}
	if inp, ok := obj["input"].(map[string]interface{}); ok {
		input = inp
	} else if inp, ok := qwenMapFromJSONString(obj["input"]); ok {
		input = inp
	} else if args, ok := obj["arguments"].(map[string]interface{}); ok {
		input = args
	} else if args, ok := qwenMapFromJSONString(obj["arguments"]); ok {
		input = args
	} else if args, ok := obj["args"].(map[string]interface{}); ok {
		input = args
	} else if args, ok := qwenMapFromJSONString(obj["args"]); ok {
		input = args
	} else if params, ok := obj["parameters"].(map[string]interface{}); ok {
		input = params
	} else if fn, ok := obj["function"].(map[string]interface{}); ok {
		if args, ok := fn["arguments"].(map[string]interface{}); ok {
			input = args
		} else if args, ok := qwenMapFromJSONString(fn["arguments"]); ok {
			input = args
		}
	}
	if input == nil {
		input = qwenTopLevelArguments(obj)
	}

	// 反混淆工具名
	name = fromQwenToolName(name)

	return &qwenParsedToolCall{
		Name:  name,
		Input: input,
	}
}

func parseToolCallJSONList(jsonStr string) []qwenParsedToolCall {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &obj); err == nil {
		if calls := qwenToolCallsFromObject(obj); len(calls) > 0 {
			return calls
		}
		if tc := parseToolCallJSON(jsonStr); tc != nil {
			return []qwenParsedToolCall{*tc}
		}
		return nil
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
		return nil
	}
	result := make([]qwenParsedToolCall, 0, len(arr))
	for _, item := range arr {
		itemBytes, _ := json.Marshal(item)
		if tc := parseToolCallJSON(string(itemBytes)); tc != nil {
			result = append(result, *tc)
		}
	}
	return result
}

func qwenToolCallsFromObject(obj map[string]interface{}) []qwenParsedToolCall {
	raw, ok := obj["tool_calls"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]qwenParsedToolCall, 0, len(raw))
	for _, item := range raw {
		itemObj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		itemBytes, _ := json.Marshal(itemObj)
		if tc := parseToolCallJSON(string(itemBytes)); tc != nil {
			result = append(result, *tc)
		}
	}
	return result
}

func qwenMapFromJSONString(value interface{}) (map[string]interface{}, bool) {
	raw, ok := value.(string)
	if !ok {
		return nil, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, false
	}
	return obj, true
}

func qwenTopLevelArguments(obj map[string]interface{}) map[string]interface{} {
	input := make(map[string]interface{})
	for key, value := range obj {
		switch key {
		case "name", "type", "id", "index", "slot", "n", "function", "tool_calls", "input", "arguments", "args", "parameters":
			continue
		}
		input[key] = value
	}
	if len(input) == 0 {
		return make(map[string]interface{})
	}
	return input
}

// ---------------------------------------------------------------------------
// OpenAI 格式转换
// ---------------------------------------------------------------------------

// qwenToolCallsToOpenAI 将解析出的 Qwen 工具调用转换为 OpenAI ChatToolCall 格式。
func qwenToolCallsToOpenAI(toolCalls []qwenParsedToolCall, allowedTools ...[]apicompat.ChatTool) []apicompat.ChatToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	var tools []apicompat.ChatTool
	if len(allowedTools) > 0 {
		tools = allowedTools[0]
	}
	result := make([]apicompat.ChatToolCall, 0, len(toolCalls))
	for i, tc := range toolCalls {
		name := qwenNormalizeToolNameForClient(tc.Name, tools)
		if len(tools) > 0 && !qwenToolNameAllowedForClient(name, tools) {
			continue
		}
		input := qwenPrepareToolInputForClient(tc, name, tools)
		argsBytes, _ := json.Marshal(input)
		idx := i
		result = append(result, apicompat.ChatToolCall{
			Index: &idx,
			ID:    fmt.Sprintf("call_%s", uuid.NewString()[:8]),
			Type:  "function",
			Function: apicompat.ChatFunctionCall{
				Name:      name,
				Arguments: string(argsBytes),
			},
		})
	}
	return result
}

func qwenPrepareToolInputForClient(tc qwenParsedToolCall, name string, tools []apicompat.ChatTool) map[string]interface{} {
	input := make(map[string]interface{}, len(tc.Input)+1)
	for key, value := range tc.Input {
		input[key] = value
	}
	if len(tools) > 0 && qwenToolNameMatches(name, qwenFindShellToolName(tools)) {
		if _, ok := input["description"]; !ok {
			input["description"] = qwenShellDescription(input)
		}
	}
	return input
}

func qwenShellDescription(input map[string]interface{}) string {
	command, _ := input["command"].(string)
	command = strings.TrimSpace(command)
	switch {
	case command == "":
		return "Run shell command"
	case strings.Contains(strings.ToLower(command), "get-content"):
		return "Read file content"
	case strings.Contains(strings.ToLower(command), "set-content") || strings.Contains(strings.ToLower(command), "writealltext"):
		return "Write file content"
	case strings.Contains(strings.ToLower(command), "get-childitem"):
		return "List directory files"
	case strings.Contains(strings.ToLower(command), "get-location"):
		return "Show current directory"
	default:
		return "Run shell command"
	}
}

func qwenToolNameAllowedForClient(name string, tools []apicompat.ChatTool) bool {
	if name == "" {
		return false
	}
	for _, t := range tools {
		if t.Function != nil && t.Function.Name == name {
			return true
		}
	}
	return false
}

func qwenFallbackFinishReason(finishReason string) *string {
	if finishReason == "tool_calls" {
		finishReason = "stop"
	}
	return &finishReason
}

func qwenNormalizeToolNameForClient(name string, tools []apicompat.ChatTool) string {
	name = fromQwenToolName(strings.TrimSpace(name))
	if name == "" || len(tools) == 0 {
		return name
	}
	if name == qwenShellToolName {
		if shellName := qwenFindShellToolName(tools); shellName != "" {
			return shellName
		}
	}
	for _, t := range tools {
		if t.Function != nil && t.Function.Name == name {
			return t.Function.Name
		}
	}
	if idx, ok := qwenClientSlotToIndex(name); ok {
		seen := -1
		for _, t := range tools {
			if t.Function == nil || t.Function.Name == "" {
				continue
			}
			seen++
			if seen == idx {
				return t.Function.Name
			}
		}
	}
	if idx, ok := qwenActionAliasToIndex(name); ok {
		seen := -1
		for _, t := range tools {
			if t.Function == nil || t.Function.Name == "" {
				continue
			}
			seen++
			if seen == idx {
				return t.Function.Name
			}
		}
	}
	nameKey := qwenToolAliasKey(name)
	if nameKey == "" {
		return name
	}
	for _, t := range tools {
		if t.Function == nil || t.Function.Name == "" {
			continue
		}
		if qwenToolAliasKey(t.Function.Name) == nameKey {
			return t.Function.Name
		}
	}
	return name
}

func qwenToolAliasKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// 流式工具调用检测器 (ToolSieve)
// ---------------------------------------------------------------------------

// qwenToolSieve 用于流式响应中实时检测工具调用标记。
type qwenToolSieve struct {
	toolNames          map[string]bool
	buffer             strings.Builder
	inToolCall         bool
	toolCallStartIndex int
	detectedToolCalls  []qwenParsedToolCall
	hasToolCalls       bool
}

// newQwenToolSieve 创建新的流式工具调用检测器。
func newQwenToolSieve(tools []apicompat.ChatTool) *qwenToolSieve {
	names := make(map[string]bool)
	for _, t := range tools {
		if t.Function != nil && t.Function.Name != "" {
			names[t.Function.Name] = true
		}
	}
	return &qwenToolSieve{toolNames: names}
}

// processChunk 处理一个文本 chunk，返回 (普通文本, 是否检测到工具调用)。
// 注意：流式场景下工具调用通常在完整响应后才能可靠解析，所以这里做简单检测，
// 最终解析在 finish 时完成。
func (s *qwenToolSieve) processChunk(chunk string) (string, bool) {
	if chunk == "" {
		return "", false
	}
	s.buffer.WriteString(chunk)
	current := s.buffer.String()

	// 如果已经检测到工具调用，不再输出普通文本
	if s.hasToolCalls {
		return "", true
	}

	// 检测是否包含工具调用标记
	if strings.Contains(current, "##TOOL_CALL##") ||
		strings.Contains(current, "<tool_call>") ||
		strings.Contains(current, "```tool_call") {
		// 尝试解析完整响应
		prefix, toolCalls, finishReason := parseQwenToolCalls(current)
		if finishReason == "tool_calls" && len(toolCalls) > 0 {
			s.detectedToolCalls = toolCalls
			s.hasToolCalls = true
			// 清空 buffer，返回前缀文本（如果有）
			s.buffer.Reset()
			return prefix, true
		}
	}

	// 还没有完整工具调用，安全地输出部分文本
	// 保留最后 50 个字符不输出，以防工具调用标记被截断
	bufStr := s.buffer.String()
	if len(bufStr) > 50 {
		safeOutput := bufStr[:len(bufStr)-50]
		s.buffer.Reset()
		s.buffer.WriteString(bufStr[len(bufStr)-50:])
		return safeOutput, false
	}

	return "", false
}

// flush 刷新剩余内容，返回最终解析结果。
func (s *qwenToolSieve) flush() (string, []qwenParsedToolCall, bool) {
	bufStr := s.buffer.String()
	if bufStr == "" {
		return "", nil, false
	}
	prefix, toolCalls, finishReason := parseQwenToolCalls(bufStr)
	if finishReason == "tool_calls" && len(toolCalls) > 0 {
		return prefix, toolCalls, true
	}
	return bufStr, nil, false
}

// ---------------------------------------------------------------------------
// 辅助：从请求体解析 messages 和 tools
// ---------------------------------------------------------------------------

// parseQwenChatRequest 解析 OpenAI Chat Completions 请求体。
type qwenChatRequest struct {
	Messages []apicompat.ChatMessage `json:"messages"`
	Tools    []apicompat.ChatTool    `json:"tools,omitempty"`
	System   string                  `json:"system,omitempty"`
}

// parseQwenRequestBody 解析请求体，提取 messages、tools 和 system prompt。
func parseQwenRequestBody(body []byte) (*qwenChatRequest, error) {
	var req qwenChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse request body: %w", err)
	}

	// 如果 system 字段为空，从 messages 中提取 system 消息
	if req.System == "" {
		for _, msg := range req.Messages {
			if msg.Role == "system" {
				req.System = extractMessageText(msg.Content)
				break
			}
		}
	}

	return &req, nil
}

// ---------------------------------------------------------------------------
// Anthropic Messages API -> OpenAI Chat Completions 转换（用于 /v1/messages 端点）
// ---------------------------------------------------------------------------

// ConvertAnthropicToChatCompletions 将 Anthropic Messages API 请求体转换为
// OpenAI Chat Completions 格式，使 Qwen 平台能统一处理。
func ConvertAnthropicToChatCompletions(body []byte) ([]byte, error) {
	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}

	// 构建 OpenAI 请求体
	openaiReq := map[string]interface{}{
		"model":  anthropicReq.Model,
		"stream": anthropicReq.Stream,
	}

	// 转换 tools
	if len(anthropicReq.Tools) > 0 {
		openaiTools := make([]apicompat.ChatTool, 0, len(anthropicReq.Tools))
		for _, t := range anthropicReq.Tools {
			openaiTools = append(openaiTools, apicompat.ChatTool{
				Type: "function",
				Function: &apicompat.ChatFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.InputSchema,
				},
			})
		}
		openaiReq["tools"] = openaiTools
	}

	// 处理 max_tokens
	if anthropicReq.MaxTokens > 0 {
		openaiReq["max_tokens"] = anthropicReq.MaxTokens
	}

	// 转换 messages
	var messages []apicompat.ChatMessage

	// 处理 system prompt
	if len(anthropicReq.System) > 0 {
		// system 可以是字符串或数组
		var systemText string
		if err := json.Unmarshal(anthropicReq.System, &systemText); err == nil {
			messages = append(messages, apicompat.ChatMessage{
				Role:    "system",
				Content: json.RawMessage(strconv.Quote(systemText)),
			})
		} else {
			// 尝试解析为数组
			var systemBlocks []apicompat.AnthropicContentBlock
			if err := json.Unmarshal(anthropicReq.System, &systemBlocks); err == nil {
				var texts []string
				for _, block := range systemBlocks {
					if block.Type == "text" {
						texts = append(texts, block.Text)
					}
				}
				if len(texts) > 0 {
					messages = append(messages, apicompat.ChatMessage{
						Role:    "system",
						Content: json.RawMessage(strconv.Quote(strings.Join(texts, "\n"))),
					})
				}
			}
		}
	}

	// 转换对话消息
	for _, msg := range anthropicReq.Messages {
		openaiMsg := apicompat.ChatMessage{
			Role: msg.Role,
		}

		// content 可以是字符串或数组
		var textContent string
		var contentBlocks []apicompat.AnthropicContentBlock

		if err := json.Unmarshal(msg.Content, &textContent); err == nil {
			// 纯文本 content
			openaiMsg.Content = json.RawMessage(strconv.Quote(textContent))
		} else if err := json.Unmarshal(msg.Content, &contentBlocks); err == nil {
			// 数组 content（可能包含 text、tool_use、tool_result）
			var contentParts []map[string]interface{}
			var toolCalls []apicompat.ChatToolCall

			for _, block := range contentBlocks {
				switch block.Type {
				case "text":
					contentParts = append(contentParts, map[string]interface{}{
						"type": "text",
						"text": block.Text,
					})
				case "image":
					if block.Source != nil && strings.TrimSpace(block.Source.Data) != "" {
						mediaType := strings.TrimSpace(block.Source.MediaType)
						if mediaType == "" {
							mediaType = "image/png"
						}
						contentParts = append(contentParts, map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": "data:" + mediaType + ";base64," + strings.TrimSpace(block.Source.Data),
							},
						})
					}
				case "document":
					if block.Source != nil && strings.TrimSpace(block.Source.Data) != "" {
						mediaType := strings.TrimSpace(block.Source.MediaType)
						if mediaType == "" {
							mediaType = "application/pdf"
						}
						filename := strings.TrimSpace(block.Filename)
						if filename == "" {
							filename = strings.TrimSpace(block.Title)
						}
						if filename == "" {
							filename = "document.pdf"
						}
						contentParts = append(contentParts, map[string]interface{}{
							"type":     "file",
							"filename": filename,
							"source": map[string]interface{}{
								"type":       "base64",
								"media_type": mediaType,
								"data":       strings.TrimSpace(block.Source.Data),
							},
						})
					}
				case "thinking":
					// 忽略 thinking 块
				case "tool_use":
					toolCalls = append(toolCalls, apicompat.ChatToolCall{
						ID:   block.ID,
						Type: "function",
						Function: apicompat.ChatFunctionCall{
							Name:      block.Name,
							Arguments: string(block.Input),
						},
					})
				case "tool_result":
					// tool_result 应该出现在 user 消息中
					if msg.Role == "user" {
						// 提取 tool_result 的 content
						var resultText string
						if err := json.Unmarshal(block.Content, &resultText); err == nil {
							// 在当前消息之前插入一个 tool 消息
							messages = append(messages, apicompat.ChatMessage{
								Role:       "tool",
								Content:    json.RawMessage(strconv.Quote(resultText)),
								ToolCallID: block.ToolUseID,
							})
						} else {
							var resultBlocks []apicompat.AnthropicContentBlock
							if err := json.Unmarshal(block.Content, &resultBlocks); err == nil {
								var resultTexts []string
								for _, rb := range resultBlocks {
									if rb.Type == "text" {
										resultTexts = append(resultTexts, rb.Text)
									}
								}
								messages = append(messages, apicompat.ChatMessage{
									Role:       "tool",
									Content:    json.RawMessage(strconv.Quote(strings.Join(resultTexts, "\n"))),
									ToolCallID: block.ToolUseID,
								})
							}
						}
					}
				}
			}

			if len(contentParts) > 0 {
				if len(contentParts) == 1 {
					if contentParts[0]["type"] == "text" {
						if text, ok := contentParts[0]["text"].(string); ok {
							openaiMsg.Content = json.RawMessage(strconv.Quote(text))
						}
					}
				}
				if openaiMsg.Content == nil {
					contentBytes, _ := json.Marshal(contentParts)
					openaiMsg.Content = contentBytes
				}
			}
			if len(toolCalls) > 0 {
				openaiMsg.ToolCalls = toolCalls
			}
		} else {
			// 无法解析，保留原样
			openaiMsg.Content = msg.Content
		}

		// 只有当消息有内容时才添加（tool_result 已经在上面处理了）
		if openaiMsg.Content != nil || len(openaiMsg.ToolCalls) > 0 {
			messages = append(messages, openaiMsg)
		}
	}

	openaiReq["messages"] = messages

	return json.Marshal(openaiReq)
}
