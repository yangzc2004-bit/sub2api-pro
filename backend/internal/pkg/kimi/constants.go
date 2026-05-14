package kimi

import (
	"net/http"
	"runtime"
	"strings"
)

const (
	CLIClientVersion = "1.5"
	CLIUserAgent     = "KimiCLI/" + CLIClientVersion
)

// Model represents a Kimi Code model option exposed to the admin UI.
type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// DefaultModels is the curated Kimi Code model list used by account creation
// and connection testing. These public aliases map to kimi-for-coding upstream.
var DefaultModels = []Model{
	{ID: "kimi-k2.6-full", Type: "model", DisplayName: "Kimi K2.6 Full", CreatedAt: ""},
	{ID: "kimi-k2.6-tools-search", Type: "model", DisplayName: "Kimi K2.6 Tools + Search", CreatedAt: ""},
	{ID: "kimi-k2.6-multimodal", Type: "model", DisplayName: "Kimi K2.6 Multimodal", CreatedAt: ""},
	{ID: "kimi-k2.6", Type: "model", DisplayName: "Kimi K2.6", CreatedAt: ""},
	{ID: "kimi-k2.6-thinking", Type: "model", DisplayName: "Kimi K2.6 Thinking", CreatedAt: ""},
	{ID: "kimi-k2.6-search", Type: "model", DisplayName: "Kimi K2.6 Search", CreatedAt: ""},
	{ID: "kimi-k2.6-thinking-search", Type: "model", DisplayName: "Kimi K2.6 Thinking + Search", CreatedAt: ""},
	{ID: "kimi-k2.6-vision", Type: "model", DisplayName: "Kimi K2.6 Vision", CreatedAt: ""},
	{ID: "kimi-k2.6-vision-thinking", Type: "model", DisplayName: "Kimi K2.6 Vision + Thinking", CreatedAt: ""},
	{ID: "kimi-k2.6-vision-search", Type: "model", DisplayName: "Kimi K2.6 Vision + Search", CreatedAt: ""},
	{ID: "kimi-k2.6-tools", Type: "model", DisplayName: "Kimi K2.6 Tools", CreatedAt: ""},
	{ID: "kimi-for-coding", Type: "model", DisplayName: "Kimi for Coding", CreatedAt: ""},
}

// DefaultTestModel is the default model to preselect in Kimi test flows.
const DefaultTestModel = "kimi-k2.6-full"

// ApplyCodingAgentHeaders makes Kimi Code requests look like a supported
// coding agent. Kimi rejects plain browser/curl/Go clients for kimi-for-coding.
func ApplyCodingAgentHeaders(header http.Header, candidateUserAgent string) {
	header.Set("User-Agent", CodingAgentUserAgent(candidateUserAgent))
	header.Set("X-Msh-Platform", "kimi_cli")
	header.Set("X-Msh-Version", CLIClientVersion)
	header.Set("X-Msh-Device-Name", "sub2api")
	header.Set("X-Msh-Device-Model", runtime.GOOS+" "+runtime.GOARCH)
	header.Set("X-Msh-Os-Version", runtime.GOOS)
}

func CodingAgentUserAgent(candidate string) string {
	ua := strings.TrimSpace(candidate)
	if IsCodingAgentUserAgent(ua) {
		return ua
	}
	return CLIUserAgent
}

func IsCodingAgentUserAgent(ua string) bool {
	lower := strings.ToLower(strings.TrimSpace(ua))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "kimicli") ||
		strings.Contains(lower, "kimi-cli") ||
		strings.Contains(lower, "kimi cli") ||
		strings.Contains(lower, "claude-cli") ||
		strings.Contains(lower, "claude code") ||
		strings.Contains(lower, "roo code") ||
		strings.Contains(lower, "roo-code") ||
		strings.Contains(lower, "kilo code") ||
		strings.Contains(lower, "kilo-code")
}
