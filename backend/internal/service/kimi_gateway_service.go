package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

var kimiCCAllowedHeaders = map[string]bool{
	"accept-language": true,
	"user-agent":      true,
}

func (s *GatewayService) ForwardKimiChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*ForwardResult, error) {
	startTime := time.Now()
	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" {
		writeGatewayCCError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	clientStream := gjson.GetBytes(body, "stream").Bool()
	reasoningEffort := extractCCReasoningEffortFromBody(body)

	upstreamModel := account.GetMappedModel(originalModel)
	upstreamBody := body
	if upstreamModel != originalModel {
		upstreamBody = s.ReplaceModelInBody(body, upstreamModel)
	}
	upstreamBody, err := prepareKimiChatCompletionsRequestBody(upstreamBody)
	if err != nil {
		return nil, fmt.Errorf("prepare kimi request body: %w", err)
	}
	if clientStream {
		var usageErr error
		upstreamBody, usageErr = ensureOpenAIChatStreamUsage(upstreamBody)
		if usageErr != nil {
			return nil, fmt.Errorf("enable stream usage: %w", usageErr)
		}
	}

	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token == "" {
		return nil, fmt.Errorf("account %d missing access_token", account.ID)
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetKimiBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAIChatCompletionsURL(baseURL)

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, clientStream)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	if clientStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	for key, values := range c.Request.Header {
		if kimiCCAllowedHeaders[strings.ToLower(key)] {
			for _, v := range values {
				upstreamReq.Header.Add(key, v)
			}
		}
	}
	kimi.ApplyCodingAgentHeaders(upstreamReq.Header, c.Request.Header.Get("User-Agent"))

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			if s.rateLimitService != nil {
				s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			return nil, &UpstreamFailoverError{
				StatusCode:   resp.StatusCode,
				ResponseBody: respBody,
			}
		}
		writeGatewayCCError(c, mapUpstreamStatusCode(resp.StatusCode), "server_error", upstreamMsg)
		return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
	}

	if clientStream {
		return s.streamKimiChatCompletions(c, resp, originalModel, upstreamModel, reasoningEffort, startTime)
	}
	return s.bufferKimiChatCompletions(c, resp, originalModel, upstreamModel, reasoningEffort, startTime)
}

func prepareKimiChatCompletionsRequestBody(body []byte) ([]byte, error) {
	if !shouldDisableKimiThinkingForToolCompat(body) || gjson.GetBytes(body, "thinking").Exists() {
		return body, nil
	}
	return sjson.SetBytes(body, "thinking", map[string]any{"type": "disabled"})
}

func shouldDisableKimiThinkingForToolCompat(body []byte) bool {
	if tools := gjson.GetBytes(body, "tools"); tools.IsArray() && len(tools.Array()) > 0 {
		return true
	}
	if gjson.GetBytes(body, "tool_choice").Exists() {
		return true
	}

	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return false
	}
	for _, message := range messages.Array() {
		if message.Get("role").String() == "tool" {
			return true
		}
		if toolCalls := message.Get("tool_calls"); toolCalls.IsArray() && len(toolCalls.Array()) > 0 {
			return true
		}
		if message.Get("function_call").Exists() {
			return true
		}
	}
	return false
}

func (s *GatewayService) streamKimiChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	var usage ClaudeUsage
	var firstTokenMs *int
	clientDisconnected := false
	skipCurrentEventSeparator := false
	for scanner.Scan() {
		line := scanner.Text()
		if payload, ok := extractOpenAISSEDataLine(line); ok {
			trimmedPayload := strings.TrimSpace(payload)
			if trimmedPayload != "[DONE]" {
				usageOnlyChunk := isOpenAIChatUsageOnlyStreamChunk(payload)
				if u := extractCCStreamUsage(payload); u != nil {
					usage.InputTokens = u.InputTokens
					usage.OutputTokens = u.OutputTokens
					usage.CacheReadInputTokens = u.CacheReadInputTokens
				}
				if firstTokenMs == nil && !usageOnlyChunk {
					elapsed := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &elapsed
				}
				normalizedPayload, keep := normalizeKimiChatCompletionsStreamPayload(payload)
				if !keep {
					skipCurrentEventSeparator = true
					continue
				}
				skipCurrentEventSeparator = false
				line = "data: " + normalizedPayload
			} else {
				skipCurrentEventSeparator = false
			}
		}
		if line == "" && skipCurrentEventSeparator {
			skipCurrentEventSeparator = false
			continue
		}
		if !clientDisconnected {
			if _, werr := c.Writer.WriteString(line + "\n"); werr != nil {
				clientDisconnected = true
				logger.L().Debug("kimi chat_completions: client disconnected, draining upstream",
					zap.Error(werr),
					zap.String("request_id", requestID),
				)
			}
		}
		if line == "" {
			if !clientDisconnected {
				c.Writer.Flush()
			}
			continue
		}
		if !clientDisconnected {
			c.Writer.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("kimi chat_completions: stream read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}
	return &ForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		Stream:          true,
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
	}, nil
}

func normalizeKimiChatCompletionsStreamPayload(payload string) (string, bool) {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" {
		return payload, true
	}
	if trimmed == "[DONE]" {
		return "[DONE]", true
	}

	var chunk apicompat.ChatCompletionsChunk
	if err := json.Unmarshal([]byte(trimmed), &chunk); err != nil {
		return payload, true
	}
	if len(chunk.Choices) == 0 {
		return "", false
	}

	kept := chunk.Choices[:0]
	for _, choice := range chunk.Choices {
		if choice.Delta.ReasoningContent != nil {
			choice.Delta.ReasoningContent = nil
		}
		if isEmptyKimiChatDelta(choice.Delta) && choice.FinishReason == nil {
			continue
		}
		kept = append(kept, choice)
	}
	if len(kept) == 0 {
		return "", false
	}
	chunk.Choices = kept

	data, err := json.Marshal(chunk)
	if err != nil {
		return payload, true
	}
	return string(data), true
}

func isEmptyKimiChatDelta(delta apicompat.ChatDelta) bool {
	return delta.Role == "" &&
		delta.Content == nil &&
		len(delta.ToolCalls) == 0
}

func (s *GatewayService) bufferKimiChatCompletions(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	upstreamModel string,
	reasoningEffort *string,
	startTime time.Time,
) (*ForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}

	var ccResp apicompat.ChatCompletionsResponse
	var usage ClaudeUsage
	if err := json.Unmarshal(respBody, &ccResp); err == nil && ccResp.Usage != nil {
		usage.InputTokens = ccResp.Usage.PromptTokens
		usage.OutputTokens = ccResp.Usage.CompletionTokens
		if ccResp.Usage.PromptTokensDetails != nil {
			usage.CacheReadInputTokens = ccResp.Usage.PromptTokensDetails.CachedTokens
		}
	}
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	} else {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(respBody)

	return &ForwardResult{
		RequestID:       requestID,
		Usage:           usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		Stream:          false,
		Duration:        time.Since(startTime),
	}, nil
}
