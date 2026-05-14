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
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var mimoPassthroughAllowedHeaders = map[string]bool{
	"accept-language": true,
	"user-agent":      true,
}

func (s *GatewayService) ForwardMimoChatCompletions(
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
	if clientStream {
		var usageErr error
		upstreamBody, usageErr = ensureOpenAIChatStreamUsage(upstreamBody)
		if usageErr != nil {
			return nil, fmt.Errorf("enable stream usage: %w", usageErr)
		}
	}

	apiKey := account.GetMimoAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetMimoOpenAIBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid mimo openai base_url: %w", err)
	}
	targetURL := buildOpenAIChatCompletionsURL(baseURL)

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, clientStream)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("api-key", apiKey)
	if clientStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	copyMimoPassthroughHeaders(c, upstreamReq)

	resp, err := s.doMimoUpstream(ctx, c, account, upstreamReq)
	if err != nil {
		writeGatewayCCError(c, http.StatusBadGateway, "server_error", "Upstream request failed")
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return s.mimoFailoverError(ctx, c, account, resp)
		}
		return nil, s.writeMimoOpenAIError(c, resp)
	}

	if clientStream {
		return s.streamMimoChatCompletions(c, resp, originalModel, upstreamModel, reasoningEffort, startTime)
	}
	return s.bufferMimoChatCompletions(c, resp, originalModel, upstreamModel, reasoningEffort, startTime)
}

func (s *GatewayService) ForwardMimoMessages(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *ParsedRequest,
) (*ForwardResult, error) {
	startTime := time.Now()
	originalModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if originalModel == "" && parsed != nil {
		originalModel = strings.TrimSpace(parsed.Model)
	}
	if originalModel == "" {
		return nil, fmt.Errorf("missing model in request")
	}
	clientStream := gjson.GetBytes(body, "stream").Bool()
	if parsed != nil {
		clientStream = parsed.Stream
	}
	upstreamModel := account.GetMappedModel(originalModel)
	upstreamBody := body
	if upstreamModel != originalModel {
		upstreamBody = s.ReplaceModelInBody(body, upstreamModel)
	}

	apiKey := account.GetMimoAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetMimoAnthropicBaseURL())
	if err != nil {
		return nil, fmt.Errorf("invalid mimo anthropic base_url: %w", err)
	}
	targetURL := buildMimoAnthropicMessagesURL(baseURL)

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, clientStream)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("api-key", apiKey)
	upstreamReq.Header.Set("anthropic-version", "2023-06-01")
	if clientStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	copyMimoPassthroughHeaders(c, upstreamReq)

	resp, err := s.doMimoUpstream(ctx, c, account, upstreamReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return s.mimoFailoverError(ctx, c, account, resp)
		}
		return s.handleErrorResponse(ctx, resp, c, account)
	}

	var usage *ClaudeUsage
	var firstTokenMs *int
	var clientDisconnect bool
	if clientStream {
		streamResult, err := s.handleStreamingResponseAnthropicAPIKeyPassthrough(ctx, resp, c, account, startTime, upstreamModel)
		if err != nil {
			return nil, err
		}
		usage = streamResult.usage
		firstTokenMs = streamResult.firstTokenMs
		clientDisconnect = streamResult.clientDisconnect
	} else {
		var err error
		usage, err = s.handleNonStreamingResponseAnthropicAPIKeyPassthrough(ctx, resp, c, account)
		if err != nil {
			return nil, err
		}
	}
	if usage == nil {
		usage = &ClaudeUsage{}
	}
	return &ForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            *usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		Stream:           clientStream,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnect,
	}, nil
}

func (s *GatewayService) doMimoUpstream(ctx context.Context, c *gin.Context, account *Account, req *http.Request) (*http.Response, error) {
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
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
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	return resp, nil
}

func (s *GatewayService) mimoFailoverError(ctx context.Context, c *gin.Context, account *Account, resp *http.Response) (*ForwardResult, error) {
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
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
		StatusCode:             resp.StatusCode,
		ResponseBody:           respBody,
		RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
	}
}

func (s *GatewayService) streamMimoChatCompletions(
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
	for scanner.Scan() {
		line := scanner.Text()
		if payload, ok := extractOpenAISSEDataLine(line); ok {
			trimmedPayload := strings.TrimSpace(payload)
			if trimmedPayload != "" && trimmedPayload != "[DONE]" {
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
			}
		}
		if !clientDisconnected {
			if _, err := c.Writer.WriteString(line + "\n"); err != nil {
				clientDisconnected = true
			}
		}
		if line == "" && !clientDisconnected {
			c.Writer.Flush()
		}
	}
	if err := scanner.Err(); err != nil && !clientDisconnected && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("stream read error: %w", err)
	}
	return &ForwardResult{
		RequestID:        requestID,
		Usage:            usage,
		Model:            originalModel,
		UpstreamModel:    upstreamModel,
		ReasoningEffort:  reasoningEffort,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnected,
	}, nil
}

func (s *GatewayService) bufferMimoChatCompletions(
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

func (s *GatewayService) writeMimoOpenAIError(c *gin.Context, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = http.StatusText(resp.StatusCode)
	}
	c.Header("Content-Type", "application/json")
	c.JSON(mapUpstreamStatusCode(resp.StatusCode), gin.H{
		"error": gin.H{
			"type":    "server_error",
			"message": upstreamMsg,
		},
	})
	return fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

func copyMimoPassthroughHeaders(c *gin.Context, upstreamReq *http.Request) {
	if c == nil || c.Request == nil {
		return
	}
	for key, values := range c.Request.Header {
		if !mimoPassthroughAllowedHeaders[strings.ToLower(strings.TrimSpace(key))] {
			continue
		}
		for _, value := range values {
			upstreamReq.Header.Add(key, value)
		}
	}
	upstreamReq.Header.Del("authorization")
	upstreamReq.Header.Del("x-api-key")
	upstreamReq.Header.Del("x-goog-api-key")
	upstreamReq.Header.Del("cookie")
}

func buildMimoAnthropicMessagesURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(normalized, "/v1/messages") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/anthropic") {
		return normalized + "/v1/messages"
	}
	return normalized + "/anthropic/v1/messages"
}
